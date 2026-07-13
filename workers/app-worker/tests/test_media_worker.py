from __future__ import annotations

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


class WorkerMediaTests(unittest.IsolatedAsyncioTestCase):
    def setUp(self) -> None:
        worker.canceled_runs.clear()

    def tearDown(self) -> None:
        worker.canceled_runs.clear()

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
