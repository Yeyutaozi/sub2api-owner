from __future__ import annotations

import json
import sys
import unittest
from pathlib import Path

import httpx


sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "src"))

from sub2api_worker.literature_search import (  # noqa: E402
    LiteratureRecord,
    LiteratureSearchClient,
    LiteratureSearchError,
    LiteratureSearchOptions,
    canonical_doi,
    format_literature_citation,
)


class LiteratureSearchTests(unittest.IsolatedAsyncioTestCase):
    async def test_auto_search_merges_openalex_and_crossref_by_doi(self) -> None:
        async def handler(request: httpx.Request) -> httpx.Response:
            if request.url.host == "api.openalex.org":
                return httpx.Response(
                    200,
                    json={
                        "results": [
                            {
                                "id": "https://openalex.org/W1",
                                "doi": "https://doi.org/10.1234/shared.2025",
                                "title": "Shared Research Title",
                                "publication_year": 2025,
                                "language": "en",
                                "authorships": [{"author": {"display_name": "Alice Example"}}],
                                "primary_location": {
                                    "landing_page_url": "https://example.test/shared",
                                    "source": {"display_name": "Example Journal"},
                                },
                                "best_oa_location": {"pdf_url": "https://papers.test/shared.pdf"},
                                "open_access": {"is_oa": True},
                                "abstract_inverted_index": {"A": [0], "useful": [1], "abstract": [2]},
                                "type": "article",
                            }
                        ]
                    },
                )
            if request.url.host == "api.crossref.org":
                return httpx.Response(
                    200,
                    json={
                        "message": {
                            "items": [
                                {
                                    "DOI": "10.1234/shared.2025",
                                    "title": ["Shared Research Title"],
                                    "author": [{"family": "Example", "given": "Alice"}],
                                    "published": {"date-parts": [[2025]]},
                                    "container-title": ["Example Journal"],
                                    "URL": "https://doi.org/10.1234/shared.2025",
                                    "type": "journal-article",
                                },
                                {
                                    "DOI": "10.5678/second.2024",
                                    "title": ["Second Research Title"],
                                    "published": {"date-parts": [[2024]]},
                                    "type": "journal-article",
                                },
                            ]
                        }
                    },
                )
            return httpx.Response(404)

        client = LiteratureSearchClient(
            transport=httpx.MockTransport(handler),
            validate_public_urls=False,
        )
        report = await client.search(
            LiteratureSearchOptions(query="research", max_results=5, provider="auto")
        )

        self.assertEqual(2, len(report.records))
        self.assertEqual(("openalex", "crossref"), report.records[0].providers)
        self.assertEqual("A useful abstract", report.records[0].abstract)
        self.assertTrue(report.records[0].pdf_url)

    async def test_auto_search_keeps_successful_provider_when_other_fails(self) -> None:
        async def handler(request: httpx.Request) -> httpx.Response:
            if request.url.host == "api.openalex.org":
                return httpx.Response(503, text="temporarily unavailable")
            return httpx.Response(
                200,
                json={
                    "message": {
                        "items": [
                            {
                                "DOI": "10.5678/fallback.2024",
                                "title": ["Fallback Result"],
                                "published": {"date-parts": [[2024]]},
                            }
                        ]
                    }
                },
            )

        client = LiteratureSearchClient(
            transport=httpx.MockTransport(handler),
            validate_public_urls=False,
            max_attempts=1,
        )
        report = await client.search(
            LiteratureSearchOptions(query="fallback", max_results=3, provider="auto")
        )

        self.assertEqual(1, len(report.records))
        self.assertEqual("openalex", report.provider_errors[0]["provider"])

    async def test_search_fails_when_no_provider_returns_results(self) -> None:
        client = LiteratureSearchClient(
            transport=httpx.MockTransport(lambda _request: httpx.Response(200, json={"results": []})),
            validate_public_urls=False,
            max_attempts=1,
        )
        with self.assertRaises(LiteratureSearchError):
            await client.search(
                LiteratureSearchOptions(query="nothing", max_results=3, provider="openalex")
            )

    async def test_download_open_access_pdf_follows_validated_redirect_and_limits_content(self) -> None:
        pdf = b"%PDF-1.7\n" + b"content" * 20

        async def handler(request: httpx.Request) -> httpx.Response:
            if request.url.path == "/start":
                return httpx.Response(302, headers={"location": "/paper.pdf"})
            return httpx.Response(200, content=pdf, headers={"content-type": "application/pdf"})

        client = LiteratureSearchClient(
            transport=httpx.MockTransport(handler),
            validate_public_urls=False,
        )
        record = LiteratureRecord(
            title="Open Paper",
            is_open_access=True,
            pdf_url="https://papers.test/start",
        )
        self.assertEqual(pdf, await client.download_open_access_pdf(record, max_bytes=4096))

        with self.assertRaisesRegex(LiteratureSearchError, "byte limit"):
            await client.download_open_access_pdf(record, max_bytes=20)

    def test_formats_citations_and_normalizes_doi(self) -> None:
        record = LiteratureRecord(
            title="A Research Article",
            authors=("Smith John",),
            year=2025,
            venue="Research Journal",
            doi="https://doi.org/10.1234/Example.2025.",
            work_type="article",
        )
        self.assertEqual("10.1234/example.2025", canonical_doi(record.doi))
        self.assertIn("[J]", format_literature_citation(record, "gbt7714_numeric"))
        self.assertIn("(2025)", format_literature_citation(record, "apa7"))


if __name__ == "__main__":
    unittest.main()
