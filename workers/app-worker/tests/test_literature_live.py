from __future__ import annotations

import os
import sys
import unittest
from pathlib import Path


sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "src"))

from sub2api_worker import main as worker  # noqa: E402
from sub2api_worker.literature_search import (  # noqa: E402
    LiteratureSearchClient,
    LiteratureSearchOptions,
)


RUN_LIVE = os.getenv("RUN_LIVE_LITERATURE_TESTS", "").lower() in {"1", "true", "yes"}


@unittest.skipUnless(RUN_LIVE, "set RUN_LIVE_LITERATURE_TESTS=1 to call public literature services")
class LiveLiteratureTests(unittest.IsolatedAsyncioTestCase):
    async def test_live_openalex_and_crossref_search(self) -> None:
        client = LiteratureSearchClient(
            timeout_seconds=30,
            mailto=os.getenv("PAPER_LITERATURE_MAILTO", ""),
            allow_proxy_fake_ip=os.getenv("PAPER_LITERATURE_ALLOW_PROXY_FAKE_IP", "").lower()
            in {"1", "true", "yes"},
        )
        report = await client.search(
            LiteratureSearchOptions(
                query="artificial intelligence early childhood education",
                max_results=3,
                provider="auto",
                from_year=2020,
                to_year=2026,
            )
        )

        self.assertTrue(report.records)
        self.assertTrue(any(record.doi for record in report.records))
        self.assertIn("openalex", report.providers)
        self.assertIn("crossref", report.providers)

    async def test_live_strict_open_access_full_text_pipeline(self) -> None:
        worker.PAPER_LITERATURE_ALLOW_PROXY_FAKE_IP = (
            os.getenv("PAPER_LITERATURE_ALLOW_PROXY_FAKE_IP", "").lower() in {"1", "true", "yes"}
        )
        payload = worker.WorkerRunRequest(
            run_id=9902,
            app_id=1,
            app_version_id=1,
            run_token="live-literature-test",
            callback_url="http://127.0.0.1/unused",
            model_proxy_url="http://127.0.0.1/unused",
            artifact_url="http://127.0.0.1/unused",
            user=worker.WorkerRunUserContext(user_id=1, api_key_id=1),
            input={
                "topic": "Scikit-learn Machine Learning in Python",
                "word_count": 1000,
                "literature_search_enabled": True,
                "literature_provider": "openalex",
                "literature_max_results": 1,
                "literature_open_access_only": True,
                "citation_evidence_enabled": True,
                "citation_style": "gbt7714_numeric",
            },
            node_model_policy={},
        )

        bundle = await worker.prepare_academic_paper_literature(
            payload,
            reference_registry=None,
            evidence_corpus=worker.EvidenceCorpus(()),
            citation_evidence_enabled=True,
            expected_reference_count=1,
        )

        self.assertEqual([1], bundle["reference_registry"]["ids"])
        self.assertEqual((1,), bundle["evidence_corpus"].reference_ids)
        self.assertEqual(1, bundle["report"]["full_text_reference_count"])
        self.assertEqual(
            "open_access_pdf_verified",
            bundle["report"]["results"][0]["full_text_status"],
        )


if __name__ == "__main__":
    unittest.main()
