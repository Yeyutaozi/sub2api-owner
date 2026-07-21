from __future__ import annotations

import asyncio
import html
import ipaddress
import os
import re
import socket
import time
import unicodedata
from dataclasses import dataclass, replace
from typing import Any, Literal
from urllib.parse import urljoin, urlparse, urlunparse

import httpx


LiteratureProvider = Literal["auto", "openalex", "crossref"]

OPENALEX_WORKS_URL = "https://api.openalex.org/works"
CROSSREF_WORKS_URL = "https://api.crossref.org/works"
_DOI_RE = re.compile(r"10\.\d{4,9}/[-._;()/:A-Z0-9]+", re.IGNORECASE)
_HTML_TAG_RE = re.compile(r"<[^>]+>")
_PROXY_FAKE_IP_NETWORK = ipaddress.ip_network("198.18.0.0/15")


class LiteratureSearchError(RuntimeError):
    pass


class LiteratureDownloadError(LiteratureSearchError):
    pass


@dataclass(frozen=True, slots=True)
class LiteratureSearchOptions:
    query: str
    max_results: int = 8
    provider: LiteratureProvider = "auto"
    from_year: int | None = None
    to_year: int | None = None
    language: str = ""
    open_access_only: bool = False

    def __post_init__(self) -> None:
        if not self.query.strip():
            raise ValueError("literature search query must not be empty")
        if self.provider not in {"auto", "openalex", "crossref"}:
            raise ValueError(f"unsupported literature provider: {self.provider}")
        if not 1 <= self.max_results <= 20:
            raise ValueError("literature max_results must be between 1 and 20")
        for value in (self.from_year, self.to_year):
            if value is not None and not 1000 <= value <= 9999:
                raise ValueError("literature year must contain four digits")
        if self.from_year is not None and self.to_year is not None and self.from_year > self.to_year:
            raise ValueError("literature from_year must not exceed to_year")


@dataclass(frozen=True, slots=True)
class LiteratureRecord:
    title: str
    authors: tuple[str, ...] = ()
    year: int | None = None
    venue: str = ""
    doi: str = ""
    url: str = ""
    abstract: str = ""
    work_type: str = "article"
    language: str = ""
    providers: tuple[str, ...] = ()
    is_open_access: bool = False
    pdf_url: str = ""

    def __post_init__(self) -> None:
        if not self.title.strip():
            raise ValueError("literature title must not be empty")

    @property
    def identity_key(self) -> str:
        doi = canonical_doi(self.doi)
        return f"doi:{doi}" if doi else f"title:{normalize_title_key(self.title)}"

    def to_public_dict(self, *, include_abstract: bool = False) -> dict[str, Any]:
        result: dict[str, Any] = {
            "title": self.title,
            "authors": list(self.authors),
            "year": self.year,
            "venue": self.venue,
            "doi": canonical_doi(self.doi) or None,
            "url": self.url or None,
            "work_type": self.work_type,
            "language": self.language or None,
            "providers": list(self.providers),
            "is_open_access": self.is_open_access,
            "full_text_available": bool(self.pdf_url),
        }
        if include_abstract:
            result["abstract"] = self.abstract or None
        return result


@dataclass(frozen=True, slots=True)
class LiteratureSearchReport:
    query: str
    records: tuple[LiteratureRecord, ...]
    providers: tuple[str, ...]
    provider_errors: tuple[dict[str, str], ...]
    duration_ms: int

    def to_public_dict(self) -> dict[str, Any]:
        return {
            "enabled": True,
            "query": self.query,
            "providers": list(self.providers),
            "provider_errors": [dict(item) for item in self.provider_errors],
            "result_count": len(self.records),
            "duration_ms": self.duration_ms,
            "results": [record.to_public_dict() for record in self.records],
        }


class LiteratureSearchClient:
    def __init__(
        self,
        *,
        timeout_seconds: float = 20,
        mailto: str = "",
        user_agent: str = "",
        transport: httpx.AsyncBaseTransport | None = None,
        validate_public_urls: bool = True,
        max_attempts: int = 3,
        allow_proxy_fake_ip: bool = False,
    ) -> None:
        self.timeout_seconds = max(float(timeout_seconds), 1.0)
        self.mailto = mailto.strip()
        self.user_agent = user_agent.strip() or _default_user_agent(self.mailto)
        self.transport = transport
        self.validate_public_urls = validate_public_urls
        self.max_attempts = max(int(max_attempts), 1)
        self.allow_proxy_fake_ip = allow_proxy_fake_ip

    async def search(self, options: LiteratureSearchOptions) -> LiteratureSearchReport:
        started = time.perf_counter()
        providers = (
            ("openalex", "crossref")
            if options.provider == "auto"
            else (options.provider,)
        )
        tasks = {
            provider: asyncio.create_task(self._search_provider(provider, options))
            for provider in providers
        }
        records: list[LiteratureRecord] = []
        errors: list[dict[str, str]] = []
        for provider, task in tasks.items():
            try:
                records.extend(await task)
            except asyncio.CancelledError:
                raise
            except Exception as exc:
                errors.append({"provider": provider, "message": _truncate(str(exc), 500)})

        merged = merge_literature_records(records, max_results=options.max_results)
        if not merged:
            detail = "; ".join(f"{item['provider']}: {item['message']}" for item in errors)
            if detail:
                raise LiteratureSearchError(f"literature providers returned no usable results ({detail})")
            raise LiteratureSearchError("literature providers returned no usable results")
        return LiteratureSearchReport(
            query=options.query.strip(),
            records=merged,
            providers=tuple(providers),
            provider_errors=tuple(errors),
            duration_ms=int((time.perf_counter() - started) * 1000),
        )

    async def download_open_access_pdf(self, record: LiteratureRecord, *, max_bytes: int) -> bytes:
        if max_bytes < 1:
            raise ValueError("literature PDF max_bytes must be positive")
        if not record.is_open_access or not record.pdf_url:
            raise LiteratureDownloadError("literature record has no open-access PDF")

        target = _prefer_https(record.pdf_url)
        headers = {"User-Agent": self.user_agent, "Accept": "application/pdf,*/*;q=0.5"}
        async with httpx.AsyncClient(
            timeout=self.timeout_seconds,
            headers=headers,
            follow_redirects=False,
            transport=self.transport,
        ) as client:
            for _redirect in range(6):
                if self.validate_public_urls:
                    await validate_public_http_url(
                        target,
                        allow_proxy_fake_ip=self.allow_proxy_fake_ip,
                    )
                async with client.stream("GET", target) as response:
                    if response.status_code in {301, 302, 303, 307, 308}:
                        location = response.headers.get("location", "").strip()
                        if not location:
                            raise LiteratureDownloadError("open-access PDF redirect has no location")
                        target = _prefer_https(urljoin(target, location))
                        continue
                    response.raise_for_status()
                    content_length = _positive_int(response.headers.get("content-length"))
                    if content_length is not None and content_length > max_bytes:
                        raise LiteratureDownloadError("open-access PDF exceeds configured byte limit")
                    chunks: list[bytes] = []
                    size = 0
                    async for chunk in response.aiter_bytes():
                        size += len(chunk)
                        if size > max_bytes:
                            raise LiteratureDownloadError("open-access PDF exceeds configured byte limit")
                        chunks.append(chunk)
                    raw = b"".join(chunks)
                    content_type = response.headers.get("content-type", "").lower()
                    if not raw.startswith(b"%PDF-") and "application/pdf" not in content_type:
                        raise LiteratureDownloadError("open-access full text is not a PDF")
                    return raw
        raise LiteratureDownloadError("open-access PDF exceeded redirect limit")

    async def _search_provider(
        self,
        provider: str,
        options: LiteratureSearchOptions,
    ) -> tuple[LiteratureRecord, ...]:
        if provider == "openalex":
            return await self._search_openalex(options)
        if provider == "crossref":
            return await self._search_crossref(options)
        raise ValueError(f"unsupported literature provider: {provider}")

    async def _search_openalex(self, options: LiteratureSearchOptions) -> tuple[LiteratureRecord, ...]:
        filters: list[str] = []
        if options.from_year is not None:
            filters.append(f"from_publication_date:{options.from_year}-01-01")
        if options.to_year is not None:
            filters.append(f"to_publication_date:{options.to_year}-12-31")
        if options.open_access_only:
            filters.append("is_oa:true")
        params: dict[str, str | int] = {
            "search": options.query.strip(),
            "per_page": min(options.max_results * 3, 50),
        }
        if filters:
            params["filter"] = ",".join(filters)
        if self.mailto:
            params["mailto"] = self.mailto

        payload = await self._get_json(OPENALEX_WORKS_URL, params=params)
        results = payload.get("results") if isinstance(payload, dict) else None
        records: list[LiteratureRecord] = []
        for item in results if isinstance(results, list) else []:
            record = _openalex_record(item)
            if record is None or not _record_matches_options(record, options):
                continue
            records.append(record)
        return tuple(records)

    async def _search_crossref(self, options: LiteratureSearchOptions) -> tuple[LiteratureRecord, ...]:
        filters: list[str] = []
        if options.from_year is not None:
            filters.append(f"from-pub-date:{options.from_year}-01-01")
        if options.to_year is not None:
            filters.append(f"until-pub-date:{options.to_year}-12-31")
        params: dict[str, str | int] = {
            "query.bibliographic": options.query.strip(),
            "rows": min(options.max_results * 3, 50),
        }
        if filters:
            params["filter"] = ",".join(filters)
        if self.mailto:
            params["mailto"] = self.mailto

        payload = await self._get_json(CROSSREF_WORKS_URL, params=params)
        message = payload.get("message") if isinstance(payload, dict) else None
        items = message.get("items") if isinstance(message, dict) else None
        records: list[LiteratureRecord] = []
        for item in items if isinstance(items, list) else []:
            record = _crossref_record(item)
            if record is None or not _record_matches_options(record, options):
                continue
            records.append(record)
        return tuple(records)

    async def _get_json(self, url: str, *, params: dict[str, str | int]) -> dict[str, Any]:
        headers = {"User-Agent": self.user_agent, "Accept": "application/json"}
        last_error: Exception | None = None
        for attempt in range(1, self.max_attempts + 1):
            try:
                async with httpx.AsyncClient(
                    timeout=self.timeout_seconds,
                    headers=headers,
                    transport=self.transport,
                ) as client:
                    response = await client.get(url, params=params)
                response.raise_for_status()
                payload = response.json()
                if not isinstance(payload, dict):
                    raise LiteratureSearchError("literature provider returned a non-object response")
                return payload
            except asyncio.CancelledError:
                raise
            except Exception as exc:
                last_error = exc
                if attempt >= self.max_attempts or not _is_transient_http_error(exc):
                    raise
                await asyncio.sleep(min(0.5 * (2 ** (attempt - 1)), 2.0))
        raise LiteratureSearchError(str(last_error or "literature request failed"))


def merge_literature_records(
    records: list[LiteratureRecord] | tuple[LiteratureRecord, ...],
    *,
    max_results: int,
) -> tuple[LiteratureRecord, ...]:
    merged: dict[str, LiteratureRecord] = {}
    order: list[str] = []
    for record in records:
        key = record.identity_key
        if not key or key == "title:":
            continue
        current = merged.get(key)
        if current is None:
            merged[key] = record
            order.append(key)
            continue
        merged[key] = _merge_record(current, record)
    return tuple(merged[key] for key in order[:max_results])


def format_literature_citation(record: LiteratureRecord, citation_style: str = "") -> str:
    authors = ", ".join(record.authors[:8]) or "Unknown author"
    year = str(record.year) if record.year is not None else "n.d."
    doi = canonical_doi(record.doi)
    style = (citation_style or "").strip().lower()
    if style in {"apa", "apa7", "apa_english", "harvard", "author_year", "gbt7714_author_year"}:
        citation = f"{authors} ({year}). {record.title}."
        if record.venue:
            citation += f" {record.venue}."
        if doi:
            citation += f" https://doi.org/{doi}"
        elif record.url:
            citation += f" {record.url}"
        return citation.strip()

    marker = _reference_type_marker(record.work_type)
    citation = f"{authors}. {record.title}[{marker}]."
    if record.venue:
        citation += f" {record.venue},"
    citation += f" {year}."
    if doi:
        citation += f" DOI: {doi}."
    elif record.url:
        citation += f" {record.url}"
    return citation.strip()


def canonical_doi(value: str) -> str:
    if not value:
        return ""
    normalized = html.unescape(value).strip()
    normalized = re.sub(r"^https?://(?:dx\.)?doi\.org/", "", normalized, flags=re.IGNORECASE)
    normalized = re.sub(r"^doi\s*[:：]\s*", "", normalized, flags=re.IGNORECASE)
    match = _DOI_RE.search(normalized)
    if match is None:
        return ""
    return match.group(0).lower().rstrip(".,;:，。；：")


def normalize_title_key(value: str) -> str:
    normalized = unicodedata.normalize("NFKC", value or "").casefold()
    return "".join(character for character in normalized if character.isalnum())


async def validate_public_http_url(value: str, *, allow_proxy_fake_ip: bool = False) -> None:
    parsed = urlparse(value)
    if parsed.scheme != "https" or not parsed.hostname or parsed.username or parsed.password:
        raise LiteratureDownloadError("open-access PDF URL must be a public HTTPS URL")
    try:
        addresses = await asyncio.to_thread(
            socket.getaddrinfo,
            parsed.hostname,
            parsed.port or 443,
            type=socket.SOCK_STREAM,
        )
    except OSError as exc:
        raise LiteratureDownloadError("open-access PDF host cannot be resolved") from exc
    if not addresses:
        raise LiteratureDownloadError("open-access PDF host cannot be resolved")
    for address in addresses:
        ip = ipaddress.ip_address(address[4][0])
        if allow_proxy_fake_ip and ip in _PROXY_FAKE_IP_NETWORK:
            continue
        if (
            ip.is_private
            or ip.is_loopback
            or ip.is_link_local
            or ip.is_multicast
            or ip.is_reserved
            or ip.is_unspecified
        ):
            raise LiteratureDownloadError("open-access PDF host resolves to a non-public address")


def _openalex_record(item: Any) -> LiteratureRecord | None:
    if not isinstance(item, dict):
        return None
    title = _clean_text(item.get("title") or item.get("display_name"))
    if not title:
        return None
    authors: list[str] = []
    for authorship in item.get("authorships") if isinstance(item.get("authorships"), list) else []:
        author = authorship.get("author") if isinstance(authorship, dict) else None
        name = _clean_text(author.get("display_name")) if isinstance(author, dict) else ""
        if name and name not in authors:
            authors.append(name)
    primary = item.get("primary_location") if isinstance(item.get("primary_location"), dict) else {}
    best_oa = item.get("best_oa_location") if isinstance(item.get("best_oa_location"), dict) else {}
    source = primary.get("source") if isinstance(primary.get("source"), dict) else {}
    open_access = item.get("open_access") if isinstance(item.get("open_access"), dict) else {}
    doi = canonical_doi(_clean_text(item.get("doi")))
    pdf_url = _clean_text(best_oa.get("pdf_url") or primary.get("pdf_url"))
    is_open_access = bool(open_access.get("is_oa") or best_oa.get("is_oa") or pdf_url)
    url = (
        (f"https://doi.org/{doi}" if doi else "")
        or _clean_text(primary.get("landing_page_url"))
        or _clean_text(item.get("id"))
    )
    return LiteratureRecord(
        title=title,
        authors=tuple(authors[:20]),
        year=_positive_int(item.get("publication_year")),
        venue=_clean_text(source.get("display_name")),
        doi=doi,
        url=url,
        abstract=_openalex_abstract(item.get("abstract_inverted_index")),
        work_type=_clean_text(item.get("type")) or "article",
        language=_clean_text(item.get("language")),
        providers=("openalex",),
        is_open_access=is_open_access,
        pdf_url=pdf_url,
    )


def _crossref_record(item: Any) -> LiteratureRecord | None:
    if not isinstance(item, dict):
        return None
    title = _first_text(item.get("title"))
    if not title:
        return None
    authors: list[str] = []
    for raw_author in item.get("author") if isinstance(item.get("author"), list) else []:
        if not isinstance(raw_author, dict):
            continue
        family = _clean_text(raw_author.get("family"))
        given = _clean_text(raw_author.get("given"))
        name = " ".join(part for part in (family, given) if part)
        if name and name not in authors:
            authors.append(name)
    links = item.get("link") if isinstance(item.get("link"), list) else []
    pdf_url = ""
    for link in links:
        if not isinstance(link, dict):
            continue
        content_type = _clean_text(link.get("content-type")).lower()
        candidate = _clean_text(link.get("URL"))
        if candidate and ("pdf" in content_type or candidate.lower().endswith(".pdf")):
            pdf_url = candidate
            break
    licenses = item.get("license") if isinstance(item.get("license"), list) else []
    is_open_access = bool(pdf_url) or any(
        "creativecommons.org" in _clean_text(license_item.get("URL")).lower()
        for license_item in licenses
        if isinstance(license_item, dict)
    )
    doi = canonical_doi(_clean_text(item.get("DOI")))
    return LiteratureRecord(
        title=title,
        authors=tuple(authors[:20]),
        year=_crossref_year(item),
        venue=_first_text(item.get("container-title")),
        doi=doi,
        url=_clean_text(item.get("URL")) or (f"https://doi.org/{doi}" if doi else ""),
        abstract=_strip_html(_clean_text(item.get("abstract"))),
        work_type=_clean_text(item.get("type")) or "article",
        language=_clean_text(item.get("language")),
        providers=("crossref",),
        is_open_access=is_open_access,
        pdf_url=pdf_url,
    )


def _record_matches_options(record: LiteratureRecord, options: LiteratureSearchOptions) -> bool:
    if options.from_year is not None and record.year is not None and record.year < options.from_year:
        return False
    if options.to_year is not None and record.year is not None and record.year > options.to_year:
        return False
    if options.language and record.language:
        expected = options.language.lower().split("-", 1)[0]
        actual = record.language.lower().split("-", 1)[0]
        if expected != actual:
            return False
    if options.open_access_only and not record.is_open_access:
        return False
    return True


def _merge_record(first: LiteratureRecord, second: LiteratureRecord) -> LiteratureRecord:
    providers = tuple(dict.fromkeys((*first.providers, *second.providers)))
    authors = first.authors if len(first.authors) >= len(second.authors) else second.authors
    return replace(
        first,
        authors=authors,
        year=first.year or second.year,
        venue=first.venue or second.venue,
        doi=canonical_doi(first.doi) or canonical_doi(second.doi),
        url=first.url or second.url,
        abstract=first.abstract or second.abstract,
        work_type=first.work_type or second.work_type,
        language=first.language or second.language,
        providers=providers,
        is_open_access=first.is_open_access or second.is_open_access,
        pdf_url=first.pdf_url or second.pdf_url,
    )


def _openalex_abstract(value: Any) -> str:
    if not isinstance(value, dict) or not value:
        return ""
    positions: dict[int, str] = {}
    for word, indexes in value.items():
        if not isinstance(word, str) or not isinstance(indexes, list):
            continue
        for index in indexes:
            if isinstance(index, int) and 0 <= index <= 10000:
                positions[index] = word
    return " ".join(positions[index] for index in sorted(positions))


def _crossref_year(item: dict[str, Any]) -> int | None:
    for key in ("published-print", "published-online", "published", "issued", "created"):
        value = item.get(key)
        if not isinstance(value, dict):
            continue
        parts = value.get("date-parts")
        if isinstance(parts, list) and parts and isinstance(parts[0], list) and parts[0]:
            year = _positive_int(parts[0][0])
            if year is not None:
                return year
    return None


def _reference_type_marker(work_type: str) -> str:
    normalized = (work_type or "").lower()
    if "book" in normalized:
        return "M"
    if "proceeding" in normalized or "conference" in normalized:
        return "C"
    if "dissertation" in normalized or "thesis" in normalized:
        return "D"
    if "report" in normalized:
        return "R"
    return "J"


def _default_user_agent(mailto: str) -> str:
    configured = os.getenv("PAPER_LITERATURE_USER_AGENT", "").strip()
    if configured:
        return configured
    if mailto:
        return f"Sub2API-App-Worker/0.4 (mailto:{mailto})"
    return "Sub2API-App-Worker/0.4"


def _prefer_https(value: str) -> str:
    parsed = urlparse((value or "").strip())
    if parsed.scheme == "http":
        parsed = parsed._replace(scheme="https", netloc=parsed.netloc)
    return urlunparse(parsed)


def _strip_html(value: str) -> str:
    return normalize_space(_HTML_TAG_RE.sub(" ", html.unescape(value or "")))


def _clean_text(value: Any) -> str:
    if isinstance(value, str):
        return normalize_space(html.unescape(value))
    return ""


def _first_text(value: Any) -> str:
    if isinstance(value, list):
        return next((_clean_text(item) for item in value if _clean_text(item)), "")
    return _clean_text(value)


def normalize_space(value: str) -> str:
    return re.sub(r"\s+", " ", value or "").strip()


def _positive_int(value: Any) -> int | None:
    if isinstance(value, bool):
        return None
    try:
        parsed = int(value)
    except (TypeError, ValueError):
        return None
    return parsed if parsed > 0 else None


def _truncate(value: str, limit: int) -> str:
    return value if len(value) <= limit else value[: max(0, limit - 3)] + "..."


def _is_transient_http_error(exc: Exception) -> bool:
    if isinstance(exc, (httpx.TimeoutException, httpx.NetworkError, httpx.RemoteProtocolError)):
        return True
    if isinstance(exc, httpx.HTTPStatusError):
        status = exc.response.status_code
        return status == 429 or 500 <= status <= 599
    return False
