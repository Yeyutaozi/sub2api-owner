from __future__ import annotations

import asyncio
import base64
import hashlib
import hmac
import json
import sys
import time
import unittest
from pathlib import Path
from unittest.mock import AsyncMock, patch

import httpx


sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "src"))

from sub2api_worker import main as worker  # noqa: E402


REAL_ASYNC_CLIENT = httpx.AsyncClient


class TrackingAsyncStream(httpx.AsyncByteStream):
    def __init__(self, chunks: list[bytes]) -> None:
        self.chunks = chunks
        self.exhausted = False

    async def __aiter__(self):
        for chunk in self.chunks:
            yield chunk
        self.exhausted = True

    async def aclose(self) -> None:
        return None


class BlockingAsyncTransport(httpx.AsyncBaseTransport):
    def __init__(self) -> None:
        self.started = asyncio.Event()

    async def handle_async_request(self, request: httpx.Request) -> httpx.Response:
        self.started.set()
        await asyncio.Event().wait()
        raise AssertionError("blocking transport should be canceled")


class WorkerMediaTests(unittest.IsolatedAsyncioTestCase):
    def setUp(self) -> None:
        worker.canceled_runs.clear()
        worker.active_model_proxy_tasks.clear()

    def tearDown(self) -> None:
        worker.canceled_runs.clear()
        worker.active_model_proxy_tasks.clear()

    def payload(
        self,
        *,
        input_values: dict[str, object] | None = None,
        artifacts: list[worker.WorkerArtifactRef] | None = None,
        timeout_seconds: int = 600,
        artifact_policy: dict[str, object] | None = None,
    ) -> worker.WorkerRunRequest:
        return worker.WorkerRunRequest(
            run_id=101,
            app_id=11,
            app_version_id=12,
            run_token="run-secret",
            callback_url="https://sub2api.test/api/v1/agent-runs/101/callback",
            model_proxy_url="https://sub2api.test/api/v1/agent-runs/101/model-proxy",
            artifact_url="https://sub2api.test/api/v1/agent-runs/101/artifacts",
            timeout_seconds=timeout_seconds,
            user={"user_id": 21, "api_key_id": 22},
            input=input_values or {},
            input_artifacts=artifacts or [],
            metadata={"artifact_policy": artifact_policy or {}},
        )

    @staticmethod
    def policy(capability: str, model: str) -> worker.SelectedPolicy:
        return worker.SelectedPolicy(
            policy_key=f"media.{capability}",
            node_id="media",
            role="generate",
            model=model,
            model_group_id=7,
            capability=capability,
        )

    @staticmethod
    def wrapped(data: dict[str, object]) -> httpx.Response:
        return httpx.Response(200, json={"code": 0, "message": "ok", "data": data})

    @staticmethod
    def client_factory(transport: httpx.MockTransport):
        def create_client(*args: object, **kwargs: object) -> httpx.AsyncClient:
            kwargs["transport"] = transport
            return REAL_ASYNC_CLIENT(*args, **kwargs)

        return create_client

    async def test_audio_speech_binary_response_is_uploaded_and_callbacked(self) -> None:
        audio = b"fake-mp3-audio"
        callbacks: list[dict[str, object]] = []
        artifact_uploads = 0

        def handle(request: httpx.Request) -> httpx.Response:
            nonlocal artifact_uploads
            if request.url.path.endswith("/model-proxy"):
                body = json.loads(request.content)
                self.assertEqual("/v1/audio/speech", body["endpoint"])
                self.assertEqual("POST", body["method"])
                self.assertEqual("tts-1", body["model"])
                self.assertEqual(7, body["group_id"])
                self.assertEqual("hello", body["request"]["input"])
                self.assertEqual("nova", body["request"]["voice"])
                self.assertEqual("run-secret", request.headers["X-Sub2API-Run-Token"])
                return self.wrapped(
                    {
                        "response": {},
                        "usage": {"input_tokens": 2},
                        "status": 200,
                        "content_type": "audio/mpeg",
                        "body_base64": base64.b64encode(audio).decode("ascii"),
                        "headers": {"Content-Disposition": 'attachment; filename="speech.mp3"'},
                    }
                )
            if request.url.path.endswith("/artifacts/upload"):
                artifact_uploads += 1
                self.assertIn("multipart/form-data", request.headers["Content-Type"])
                self.assertIn(audio, request.content)
                self.assertIn(hashlib.sha256(audio).hexdigest().encode(), request.content)
                return self.wrapped({"artifact_id": 501, "url": "https://download.test/speech.mp3"})
            if request.url.path.endswith("/callback"):
                callbacks.append(json.loads(request.content))
                return self.wrapped({"id": 101, "status": "running"})
            self.fail(f"unexpected request: {request.method} {request.url}")

        payload = self.payload(input_values={"prompt": "hello", "voice": "nova", "response_format": "mp3"})
        transport = httpx.MockTransport(handle)
        with patch.object(worker.httpx, "AsyncClient", side_effect=self.client_factory(transport)):
            await worker.process_audio_speech_run(payload, "worker-audio", time.perf_counter(), self.policy("audio_speech", "tts-1"), "hello")

        self.assertEqual(1, artifact_uploads)
        self.assertEqual("succeeded", callbacks[-1]["event_type"])
        self.assertEqual(501, callbacks[-1]["output"]["artifact"]["artifact_id"])
        self.assertEqual({"input_tokens": 2}, callbacks[-1]["metadata"]["usage"])

    async def test_image_generation_without_reference_uses_generations_endpoint(self) -> None:
        proxy_bodies: list[dict[str, object]] = []

        def handle(request: httpx.Request) -> httpx.Response:
            body = json.loads(request.content)
            proxy_bodies.append(body)
            return self.wrapped({"response": {"data": [{"b64_json": "aW1hZ2U="}]}})

        payload = self.payload(input_values={"prompt": "a mountain", "size": "1024x1024", "quality": "high"})
        transport = httpx.MockTransport(handle)
        with patch.object(worker.httpx, "AsyncClient", side_effect=self.client_factory(transport)):
            await worker.call_image_model_proxy(payload, self.policy("image_generation", "gpt-image-1"), "a mountain")

        self.assertEqual(1, len(proxy_bodies))
        self.assertEqual("/v1/images/generations", proxy_bodies[0]["endpoint"])
        self.assertEqual("1024x1024", proxy_bodies[0]["request"]["size"])
        self.assertEqual("high", proxy_bodies[0]["request"]["quality"])
        self.assertNotIn("multipart", proxy_bodies[0])

    async def test_image_edit_forwards_multiple_reference_images_and_keeps_single_output(self) -> None:
        first = worker.WorkerArtifactRef(name="first.png", mime_type="image/png")
        second = worker.WorkerArtifactRef(name="second.webp", mime_type="image/webp")
        proxy_body: dict[str, object] = {}

        def handle(request: httpx.Request) -> httpx.Response:
            nonlocal proxy_body
            proxy_body = json.loads(request.content)
            return self.wrapped({"response": {"data": [{"b64_json": "aW1hZ2U="}]}})

        transport = httpx.MockTransport(handle)
        with patch.object(worker.httpx, "AsyncClient", side_effect=self.client_factory(transport)):
            await worker.call_image_model_proxy(
                self.payload(input_values={"prompt": "combine them"}),
                self.policy("image_generation", "gpt-image-1"),
                "combine them",
                references=[first, second],
                reference_bodies=[b"first-image", b"second-image"],
            )

        self.assertEqual("/v1/images/edits", proxy_body["endpoint"])
        self.assertEqual(1, proxy_body["request"]["n"])
        multipart = proxy_body["multipart"]
        self.assertEqual(2, len(multipart))
        self.assertEqual(["image", "image"], [part["name"] for part in multipart])
        self.assertEqual(["first.png", "second.webp"], [part["filename"] for part in multipart])
        self.assertEqual(b"first-image", base64.b64decode(multipart[0]["body_base64"]))
        self.assertEqual(b"second-image", base64.b64decode(multipart[1]["body_base64"]))

    async def test_image_edit_with_reference_uses_same_run_and_reports_mode(self) -> None:
        reference_bytes = b"fake-reference-image"
        generated_bytes = b"fake-generated-image"
        callbacks: list[dict[str, object]] = []
        reference = worker.WorkerArtifactRef(
            artifact_id=61,
            name="reference.webp",
            mime_type="image/webp",
            url="https://assets.test/reference.webp",
            metadata={"field_name": "reference_image", "asset_role": "reference"},
        )

        def handle(request: httpx.Request) -> httpx.Response:
            if request.url.host == "assets.test":
                return httpx.Response(200, content=reference_bytes, headers={"Content-Type": "image/webp"})
            if request.url.path.endswith("/model-proxy"):
                body = json.loads(request.content)
                self.assertEqual("/v1/images/edits", body["endpoint"])
                self.assertEqual("multipart/form-data", body["content_type"])
                self.assertEqual("image", body["multipart"][0]["name"])
                self.assertEqual("reference.webp", body["multipart"][0]["filename"])
                self.assertEqual("image/webp", body["multipart"][0]["content_type"])
                self.assertEqual(reference_bytes, base64.b64decode(body["multipart"][0]["body_base64"]))
                return self.wrapped(
                    {
                        "response": {"data": [{"b64_json": base64.b64encode(generated_bytes).decode("ascii")}]},
                        "usage": {"total_tokens": 12},
                    }
                )
            if request.url.path.endswith("/artifacts/upload"):
                self.assertIn(generated_bytes, request.content)
                self.assertIn(b'image_to_image', request.content)
                return self.wrapped({"artifact_id": 601, "url": "https://download.test/generated.png"})
            if request.url.path.endswith("/callback"):
                callbacks.append(json.loads(request.content))
                return self.wrapped({"id": 101, "status": "running"})
            self.fail(f"unexpected request: {request.method} {request.url}")

        payload = self.payload(input_values={"prompt": "turn it into a sketch", "input_fidelity": "high"}, artifacts=[reference])
        transport = httpx.MockTransport(handle)
        with patch.object(worker.httpx, "AsyncClient", side_effect=self.client_factory(transport)):
            await worker.process_image_run(
                payload,
                "worker-image-edit",
                time.perf_counter(),
                self.policy("image_generation", "gpt-image-1"),
                "turn it into a sketch",
            )

        self.assertEqual("succeeded", callbacks[-1]["event_type"])
        self.assertEqual("image_to_image", callbacks[-1]["output"]["generation_mode"])
        self.assertEqual(1, callbacks[-1]["output"]["reference_count"])
        self.assertEqual("image_to_image", callbacks[-1]["metadata"]["generation_mode"])
        self.assertEqual(1, callbacks[-1]["metadata"]["reference_count"])
        self.assertEqual(601, callbacks[-1]["output"]["artifact"]["artifact_id"])

    def test_reference_image_prefers_reference_role_then_falls_back(self) -> None:
        fallback = worker.WorkerArtifactRef(name="first.png", mime_type="image/png")
        preferred = worker.WorkerArtifactRef(
            name="preferred.png",
            mime_type="image/png",
            metadata={"asset_role": "reference"},
        )
        payload = self.payload(artifacts=[fallback, preferred])
        self.assertIs(preferred, worker.reference_image_artifact(payload))
        self.assertEqual([preferred], worker.reference_image_artifacts(payload))
        self.assertIs(fallback, worker.reference_image_artifact(self.payload(artifacts=[fallback])))
        self.assertIsNone(worker.reference_image_artifact(self.payload()))

        another_preferred = worker.WorkerArtifactRef(
            name="preferred-2.png",
            mime_type="image/png",
            metadata={"field_name": "reference_image", "asset_role": "reference"},
        )
        self.assertEqual(
            [preferred, another_preferred],
            worker.reference_image_artifacts(self.payload(artifacts=[fallback, preferred, another_preferred])),
        )

    async def test_multiple_reference_images_enforce_combined_size_limit(self) -> None:
        references = [
            worker.WorkerArtifactRef(name="first.png", mime_type="image/png", url="https://assets.test/first.png"),
            worker.WorkerArtifactRef(name="second.png", mime_type="image/png", url="https://assets.test/second.png"),
        ]

        def handle(_: httpx.Request) -> httpx.Response:
            return httpx.Response(200, content=b"12", headers={"Content-Type": "image/png"})

        transport = httpx.MockTransport(handle)
        with (
            patch.object(worker.httpx, "AsyncClient", side_effect=self.client_factory(transport)),
            patch.object(worker, "MAX_IMAGE_REFERENCE_BYTES", 10),
            patch.object(worker, "MAX_IMAGE_REFERENCE_TOTAL_BYTES", 3),
        ):
            with self.assertRaises(worker.WorkerFailure) as size_error:
                await worker.download_reference_images(references)
        self.assertEqual("IMAGE_REFERENCE_TOTAL_TOO_LARGE", size_error.exception.code)

    async def test_audio_transcription_builds_model_proxy_multipart_contract(self) -> None:
        audio = b"RIFF-fake-wave"
        callbacks: list[dict[str, object]] = []
        source = worker.WorkerArtifactRef(
            artifact_id=41,
            name="sample.wav",
            mime_type="audio/wav",
            url="https://assets.test/sample.wav",
        )

        def handle(request: httpx.Request) -> httpx.Response:
            if request.url.host == "assets.test":
                return httpx.Response(200, content=audio, headers={"Content-Type": "audio/wav"})
            if request.url.path.endswith("/model-proxy"):
                body = json.loads(request.content)
                self.assertEqual("/v1/audio/transcriptions", body["endpoint"])
                self.assertEqual("multipart/form-data", body["content_type"])
                self.assertEqual("zh", body["request"]["language"])
                self.assertEqual(0.2, body["request"]["temperature"])
                self.assertEqual("file", body["multipart"][0]["name"])
                self.assertEqual("sample.wav", body["multipart"][0]["filename"])
                self.assertEqual("audio/wav", body["multipart"][0]["content_type"])
                self.assertEqual(audio, base64.b64decode(body["multipart"][0]["body_base64"]))
                return self.wrapped({"response": {"text": "你好"}, "usage": {"input_tokens": 8}})
            if request.url.path.endswith("/callback"):
                callbacks.append(json.loads(request.content))
                return self.wrapped({"id": 101, "status": "running"})
            self.fail(f"unexpected request: {request.method} {request.url}")

        payload = self.payload(input_values={"language": "zh", "temperature": 0.2}, artifacts=[source])
        transport = httpx.MockTransport(handle)
        with patch.object(worker.httpx, "AsyncClient", side_effect=self.client_factory(transport)):
            await worker.process_audio_text_run(
                payload,
                "worker-transcription",
                time.perf_counter(),
                self.policy("audio_transcription", "whisper-1"),
                "audio_transcription",
            )

        self.assertEqual("succeeded", callbacks[-1]["event_type"])
        self.assertEqual("你好", callbacks[-1]["output"]["result"])

    async def test_video_reference_create_poll_content_and_artifact_upload(self) -> None:
        reference_bytes = b"fake-png-reference"
        video_bytes = b"fake-mp4-video"
        proxy_calls: list[dict[str, object]] = []
        callbacks: list[dict[str, object]] = []
        reference = worker.WorkerArtifactRef(
            artifact_id=42,
            name="reference.png",
            mime_type="image/png",
            url="https://assets.test/reference.png",
        )

        def handle(request: httpx.Request) -> httpx.Response:
            if request.url.host == "assets.test":
                return httpx.Response(200, content=reference_bytes, headers={"Content-Type": "image/png"})
            if request.url.path.endswith("/model-proxy"):
                body = json.loads(request.content)
                proxy_calls.append(body)
                endpoint = body["endpoint"]
                if endpoint == "/v1/videos":
                    self.assertEqual("multipart/form-data", body["content_type"])
                    self.assertEqual(8, body["request"]["seconds"])
                    self.assertEqual("1280x720", body["request"]["size"])
                    self.assertEqual("input_reference", body["multipart"][0]["name"])
                    self.assertEqual(reference_bytes, base64.b64decode(body["multipart"][0]["body_base64"]))
                    return self.wrapped({"response": {"id": "video_123", "status": "queued"}, "usage": {"video_count": 1}})
                if endpoint == "/v1/videos/video_123":
                    self.assertEqual("GET", body["method"])
                    return self.wrapped(
                        {
                            "response": {
                                "id": "video_123",
                                "status": "completed",
                                "url": "https://private-upstream.test/video_123",
                            }
                        }
                    )
                if endpoint == "/v1/videos/video_123/content":
                    self.assertEqual("GET", body["method"])
                    return self.wrapped(
                        {
                            "response": {},
                            "content_type": "video/mp4",
                            "body_base64": base64.b64encode(video_bytes).decode("ascii"),
                            "headers": {"Content-Disposition": 'attachment; filename="result.mp4"'},
                        }
                    )
            if request.url.path.endswith("/artifacts/upload"):
                self.assertIn(video_bytes, request.content)
                return self.wrapped({"artifact_id": 502, "url": "https://download.test/result.mp4"})
            if request.url.path.endswith("/callback"):
                callbacks.append(json.loads(request.content))
                return self.wrapped({"id": 101, "status": "running"})
            self.fail(f"unexpected request: {request.method} {request.url}")

        payload = self.payload(
            input_values={"prompt": "a sunrise", "duration": 8, "resolution": "1280x720"},
            artifacts=[reference],
            timeout_seconds=900,
        )
        transport = httpx.MockTransport(handle)
        with (
            patch.object(worker.httpx, "AsyncClient", side_effect=self.client_factory(transport)),
            patch.object(worker.asyncio, "sleep", new=AsyncMock()),
        ):
            await worker.process_video_run(payload, "worker-video", time.perf_counter(), self.policy("video_generation", "sora-2"), "a sunrise")

        self.assertEqual(
            ["/v1/videos", "/v1/videos/video_123", "/v1/videos/video_123/content"],
            [call["endpoint"] for call in proxy_calls],
        )
        self.assertEqual("succeeded", callbacks[-1]["event_type"])
        self.assertEqual("video_123", callbacks[-1]["output"]["video_id"])
        self.assertEqual(502, callbacks[-1]["output"]["artifact"]["artifact_id"])

    async def test_video_cancel_during_poll_sleep_stops_before_next_proxy_call(self) -> None:
        proxy_endpoints: list[str] = []
        callbacks: list[dict[str, object]] = []

        def handle(request: httpx.Request) -> httpx.Response:
            if request.url.path.endswith("/model-proxy"):
                body = json.loads(request.content)
                proxy_endpoints.append(body["endpoint"])
                return self.wrapped({"response": {"id": "video_cancel", "status": "queued"}})
            if request.url.path.endswith("/callback"):
                callbacks.append(json.loads(request.content))
                return self.wrapped({"id": 101, "status": "canceled"})
            self.fail(f"unexpected request: {request.method} {request.url}")

        payload = self.payload(input_values={"prompt": "cancel me"})

        async def cancel_during_sleep(_: float) -> None:
            worker.canceled_runs.add(payload.run_id)

        transport = httpx.MockTransport(handle)
        with (
            patch.object(worker.httpx, "AsyncClient", side_effect=self.client_factory(transport)),
            patch.object(worker.asyncio, "sleep", side_effect=cancel_during_sleep),
        ):
            await worker.process_video_run(payload, "worker-cancel", time.perf_counter(), self.policy("video_generation", "sora-2"), "cancel me")

        self.assertEqual(["/v1/videos"], proxy_endpoints)
        self.assertEqual("canceled", callbacks[-1]["event_type"])

    async def test_download_and_base64_limits_fail_before_upload(self) -> None:
        def handle(_: httpx.Request) -> httpx.Response:
            return httpx.Response(200, content=b"1234")

        transport = httpx.MockTransport(handle)
        source = worker.WorkerArtifactRef(name="too-large.wav", mime_type="audio/wav", url="https://assets.test/large")
        with (
            patch.object(worker.httpx, "AsyncClient", side_effect=self.client_factory(transport)),
            patch.object(worker, "MAX_MODEL_PROXY_ASSET_BYTES", 3),
        ):
            with self.assertRaises(worker.WorkerFailure) as download_error:
                await worker.download_input_artifact(source)
        self.assertEqual("INPUT_ASSET_TOO_LARGE", download_error.exception.code)

        payload = self.payload()
        with patch.object(worker, "MAX_REMOTE_ARTIFACT_BYTES", 3):
            with self.assertRaises(worker.WorkerFailure) as size_error:
                await worker.upload_base64_artifact(
                    payload,
                    name="large.bin",
                    b64_json=base64.b64encode(b"1234").decode("ascii"),
                    mime_type="application/octet-stream",
                    metadata={},
                )
        self.assertEqual("ARTIFACT_TOO_LARGE", size_error.exception.code)

        with self.assertRaises(worker.WorkerFailure) as base64_error:
            await worker.upload_base64_artifact(
                payload,
                name="invalid.bin",
                b64_json="!!!!",
                mime_type="application/octet-stream",
                metadata={},
            )
        self.assertEqual("ARTIFACT_BASE64_INVALID", base64_error.exception.code)

    async def test_failed_callback_uses_standard_error_contract(self) -> None:
        callback_body: dict[str, object] = {}

        def handle(request: httpx.Request) -> httpx.Response:
            nonlocal callback_body
            callback_body = json.loads(request.content)
            return self.wrapped({"id": 101, "status": "failed"})

        transport = httpx.MockTransport(handle)
        with patch.object(worker.httpx, "AsyncClient", side_effect=self.client_factory(transport)):
            await worker.callback_failure(self.payload(), "VIDEO_GENERATION_FAILED", "upstream failed")

        self.assertEqual("failed", callback_body["status"])
        self.assertEqual(
            {"code": "VIDEO_GENERATION_FAILED", "message": "upstream failed"},
            callback_body["error"],
        )
        self.assertEqual("VIDEO_GENERATION_FAILED", callback_body["metadata"]["error_code"])

    async def test_call_model_proxy_collects_sse_text_and_usage(self) -> None:
        stream = TrackingAsyncStream(
            [
                b'data: {"choices":[{"delta":{"content":"Hello"}}]}\n\n',
                b'data: {"choices":[{"delta":{"content":" world"}}]}\n\n',
                b'data: {"choices":[],"usage":{"prompt_tokens":2,"completion_tokens":2,"total_tokens":4}}\n\n',
                b"data: [DONE]\n\n",
                b": stream closed\n\n",
            ]
        )

        def handle(request: httpx.Request) -> httpx.Response:
            body = json.loads(request.content)
            self.assertTrue(body["request"]["stream"])
            self.assertTrue(body["request"]["stream_options"]["include_usage"])
            self.assertEqual("text/event-stream", request.headers["Accept"])
            return httpx.Response(200, headers={"Content-Type": "text/event-stream"}, stream=stream)

        on_text = AsyncMock()
        transport = httpx.MockTransport(handle)
        with patch.object(worker.httpx, "AsyncClient", side_effect=self.client_factory(transport)):
            result = await worker.call_model_proxy(
                self.payload(input_values={"prompt": "hello"}),
                worker.SelectedPolicy(
                    policy_key="text.generate",
                    node_id="text",
                    role="generate",
                    model="gpt-5-mini",
                    capability="text",
                ),
                "hello",
                on_text=on_text,
            )

        self.assertEqual("Hello world", result["response"]["text"])
        self.assertEqual(4, result["usage"]["total_tokens"])
        self.assertEqual(2, on_text.await_count)
        self.assertEqual("Hello world", on_text.await_args_list[-1].args[0])
        self.assertTrue(stream.exhausted)

    async def test_process_run_batches_partial_text_callbacks(self) -> None:
        callbacks: list[dict[str, object]] = []
        payload = self.payload(input_values={"prompt": "hello"})
        payload.node_model_policy = {
            "text.generate": worker.ModelPolicy(
                node_id="text",
                role="generate",
                model="gpt-5-mini",
                capability="text",
                required=True,
            )
        }

        async def capture_callback(_: worker.WorkerRunRequest, event_type: str, **kwargs: object) -> None:
            callbacks.append({"event_type": event_type, **kwargs})

        async def fake_model_proxy(
            _: worker.WorkerRunRequest,
            __: worker.SelectedPolicy,
            ___: str,
            *,
            on_text=None,
        ) -> dict[str, object]:
            self.assertIsNotNone(on_text)
            await on_text("Hello")
            await on_text("Hello world")
            return {
                "response": {"text": "Hello world"},
                "usage": {"total_tokens": 4},
                "metadata": {"stream": True},
            }

        with (
            patch.object(worker, "callback", side_effect=capture_callback),
            patch.object(worker, "call_model_proxy", side_effect=fake_model_proxy),
            patch.object(worker.time, "monotonic", side_effect=[0.0, 2.0, 2.1]),
        ):
            await worker.process_run(payload, "worker-stream")

        progress_callbacks = [item for item in callbacks if item["event_type"] == "progress" and item.get("output")]
        self.assertEqual(1, len(progress_callbacks))
        progress_output = progress_callbacks[0]["output"]
        self.assertIsInstance(progress_output, dict)
        self.assertEqual("Hello", progress_output["result"])
        self.assertTrue(progress_output["partial"])
        self.assertEqual("succeeded", callbacks[-1]["event_type"])
        final_output = callbacks[-1]["output"]
        self.assertIsInstance(final_output, dict)
        self.assertEqual("Hello world", final_output["result"])

    async def test_model_proxy_wait_is_canceled_before_response_headers(self) -> None:
        payload = self.payload(input_values={"prompt": "hello"})
        transport = BlockingAsyncTransport()
        client_factory = self.client_factory(transport)
        with patch.object(worker.httpx, "AsyncClient", side_effect=client_factory):
            task = asyncio.create_task(
                worker.call_model_proxy(
                    payload,
                    worker.SelectedPolicy(
                        policy_key="text.generate",
                        node_id="text",
                        role="generate",
                        model="gpt-5-mini",
                        capability="text",
                    ),
                    "hello",
                )
            )
            await transport.started.wait()
            worker.canceled_runs.add(payload.run_id)
            worker.cancel_active_model_proxy_task(payload.run_id)
            with self.assertRaises(worker.WorkerCanceled):
                await task

        self.assertNotIn(payload.run_id, worker.active_model_proxy_tasks)

    def test_official_media_capability_aliases_are_selected(self) -> None:
        payload = self.payload()
        payload.node_model_policy = {
            "speech.transcribe": worker.ModelPolicy(
                node_id="speech",
                role="transcribe",
                model="whisper-1",
                capability="audio_transcriptions",
            )
        }
        media_kind, selected = worker.select_media_policy(payload) or ("", None)
        self.assertEqual("audio_transcription", media_kind)
        self.assertIsNotNone(selected)

        payload.node_model_policy = {
            "image.edit": worker.ModelPolicy(
                node_id="image",
                role="edit",
                model="gpt-image-1",
                capability="image_to_image",
            )
        }
        media_kind, selected = worker.select_media_policy(payload) or ("", None)
        self.assertEqual("image_generation", media_kind)
        self.assertIsNotNone(selected)


class WorkerEndpointTests(unittest.IsolatedAsyncioTestCase):
    def tearDown(self) -> None:
        worker.canceled_runs.clear()

    async def test_cancel_route_verifies_hmac_signature(self) -> None:
        body = json.dumps(
            {"run_id": 202, "run_token": "cancel-secret", "reason": "user stopped"},
            separators=(",", ":"),
        ).encode()
        timestamp = str(int(time.time()))
        signature = "sha256=" + hmac.new(
            b"cancel-secret",
            timestamp.encode() + b"." + body,
            hashlib.sha256,
        ).hexdigest()

        transport = httpx.ASGITransport(app=worker.app)
        async with REAL_ASYNC_CLIENT(transport=transport, base_url="http://testserver") as client:
            response = await client.post(
                "/cancel",
                content=body,
                headers={"X-Sub2API-Timestamp": timestamp, "X-Sub2API-Signature": signature},
            )
            rejected = await client.post("/cancel", content=body)

        self.assertEqual(200, response.status_code)
        self.assertTrue(response.json()["accepted"])
        self.assertIn(202, worker.canceled_runs)
        self.assertEqual(401, rejected.status_code)


if __name__ == "__main__":
    unittest.main()
