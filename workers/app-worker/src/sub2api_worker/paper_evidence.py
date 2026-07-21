from __future__ import annotations

import hashlib
import io
import os
import re
import unicodedata
import zipfile
from dataclasses import dataclass, replace
from typing import Iterable, Literal, Mapping


DEFAULT_MIN_EVIDENCE_QUOTE_CHARS = 12
EVIDENCE_BLOCK_TARGET_CHARS = 3_000
EVIDENCE_BLOCK_OVERLAP_CHARS = 400
MAX_EVIDENCE_PDF_PAGES = 500
MAX_EVIDENCE_DOCX_ZIP_MEMBERS = 2_048
MAX_EVIDENCE_DOCX_EXPANDED_BYTES = 64 * 1024 * 1024
MAX_EVIDENCE_CHARS_PER_SOURCE = 2_000_000
MAX_EVIDENCE_CHARS_TOTAL = 5_000_000
MAX_EVIDENCE_BLOCKS_PER_SOURCE = 2_000
MAX_EVIDENCE_BLOCKS_TOTAL = 5_000
MAX_EVIDENCE_IDENTITY_OPENING_CHARS = 30_000
MAX_EVIDENCE_IDENTITY_OPENING_LOCATORS = {
    "page": 3,
    "paragraph": 30,
    "line": 80,
}

LocatorKind = Literal["page", "paragraph", "line"]
EvidenceQuoteStatus = Literal[
    "matched",
    "empty_quote",
    "quote_too_short",
    "reference_not_found",
    "reference_mismatch",
    "not_found",
    "ambiguous",
]
EvidenceAuditStatus = Literal[
    "matched",
    "missing_assertion",
    "duplicate_assertion",
    "unknown_chunk",
    "reference_mismatch",
    "quote_too_short",
    "quote_not_found",
]

_REFERENCE_SECTION_RE = re.compile(
    r"^\s*(?:source|reference|ref|来源|资料|文献)\s*"
    r"[\[【(（]\s*(\d+)\s*[\]】)）]\s*(?:(?:[:：])\s*)?(.*?)\s*$",
    re.IGNORECASE,
)
_REFERENCE_FILE_PATTERNS = (
    re.compile(r"^\s*[\[【(（]\s*(\d+)\s*[\]】)）]"),
    re.compile(r"^\s*(\d+)\s*[-_.、]"),
    re.compile(r"(?:^|[-_.\s])(?:ref(?:erence)?|source)[-_.\s]*(\d+)(?:\D|$)", re.IGNORECASE),
)
_OUTER_QUOTE_PAIRS = {
    '"': '"',
    "'": "'",
    "“": "”",
    "‘": "’",
    "「": "」",
    "『": "』",
}
_DOI_RE = re.compile(r"(?<![A-Za-z0-9])10\.\d{4,9}/[-._;()/:A-Z0-9]+", re.IGNORECASE)
_REFERENCE_TYPE_MARKER_RE = re.compile(
    r"\[(?:J|M|C|D|R|N|P|S|G|K|DB|CP|EB/OL|J/OL|M/OL)\]",
    re.IGNORECASE,
)
_REFERENCE_PUBLICATION_RE = re.compile(
    r"(?:journal|proceedings|press|publisher|review|transactions|学报|期刊|杂志|出版社|会议论文集)",
    re.IGNORECASE,
)


class EvidenceExtractionError(ValueError):
    """Raised when a supported evidence artifact cannot be parsed."""


class UnsupportedEvidenceFormat(EvidenceExtractionError):
    """Raised when an artifact is not a supported PDF, DOCX, or text file."""


class EvidenceSourceIdentityError(EvidenceExtractionError):
    """Raised when an uploaded source cannot be tied to its bibliography entry."""


@dataclass(frozen=True, slots=True)
class EvidenceLocator:
    kind: LocatorKind
    index: int
    part: int | None = None

    def __post_init__(self) -> None:
        if self.kind not in {"page", "paragraph", "line"}:
            raise ValueError(f"unsupported evidence locator kind: {self.kind}")
        if self.index < 1:
            raise ValueError("evidence locator index must be positive")
        if self.part is not None and self.part < 1:
            raise ValueError("evidence locator part must be positive")

    @property
    def label(self) -> str:
        label = f"{self.kind} {self.index}"
        if self.part is not None:
            label += f", part {self.part}"
        return label

    def to_dict(self) -> dict[str, object]:
        result: dict[str, object] = {
            "kind": self.kind,
            "index": self.index,
            "label": self.label,
        }
        if self.part is not None:
            result["part"] = self.part
        return result


@dataclass(frozen=True, slots=True)
class EvidenceSource:
    artifact_name: str
    data: bytes
    mime_type: str = ""
    reference_id: int | None = None
    artifact_id: int | str | None = None

    def __post_init__(self) -> None:
        if not self.artifact_name.strip():
            raise ValueError("evidence artifact_name must not be empty")
        if not isinstance(self.data, bytes):
            raise TypeError("evidence source data must be bytes")
        _validate_reference_id(self.reference_id)


@dataclass(frozen=True, slots=True)
class EvidenceBlock:
    chunk_id: str
    reference_id: int | None
    artifact_name: str
    source_id: str
    locator: EvidenceLocator
    text: str
    normalized_text: str

    def to_dict(self) -> dict[str, object]:
        return {
            "chunk_id": self.chunk_id,
            "reference_id": self.reference_id,
            "artifact_name": self.artifact_name,
            "locator": self.locator.to_dict(),
            "text": self.text,
            "normalized_text": self.normalized_text,
        }


@dataclass(frozen=True, slots=True)
class EvidenceCorpus:
    blocks: tuple[EvidenceBlock, ...]

    def __post_init__(self) -> None:
        chunk_ids = [block.chunk_id for block in self.blocks]
        if len(chunk_ids) != len(set(chunk_ids)):
            raise ValueError(
                "evidence chunk IDs are not unique; provide distinct artifact_id values for same-named artifacts"
            )

    @property
    def reference_ids(self) -> tuple[int, ...]:
        return tuple(sorted({block.reference_id for block in self.blocks if block.reference_id is not None}))

    def blocks_for_reference(self, reference_id: int) -> tuple[EvidenceBlock, ...]:
        _validate_reference_id(reference_id)
        return tuple(block for block in self.blocks if block.reference_id == reference_id)

    def block_by_id(self, chunk_id: str) -> EvidenceBlock | None:
        return next((block for block in self.blocks if block.chunk_id == chunk_id), None)

    def to_dict(self) -> dict[str, object]:
        return {
            "reference_ids": list(self.reference_ids),
            "chunk_count": len(self.blocks),
            "chunks": [block.to_dict() for block in self.blocks],
        }


@dataclass(frozen=True, slots=True)
class EvidenceQuoteMatch:
    valid: bool
    status: EvidenceQuoteStatus
    quote: str
    normalized_quote: str
    requested_reference_id: int | None
    resolved_reference_id: int | None
    matches: tuple[EvidenceBlock, ...] = ()

    def to_dict(self) -> dict[str, object]:
        return {
            "valid": self.valid,
            "status": self.status,
            "quote": self.quote,
            "normalized_quote": self.normalized_quote,
            "requested_reference_id": self.requested_reference_id,
            "resolved_reference_id": self.resolved_reference_id,
            "matches": [block.to_dict() for block in self.matches],
        }


@dataclass(frozen=True, slots=True)
class CitationOccurrence:
    occurrence_id: str
    reference_id: int

    def __post_init__(self) -> None:
        if not self.occurrence_id.strip():
            raise ValueError("citation occurrence_id must not be empty")
        _validate_reference_id(self.reference_id)


@dataclass(frozen=True, slots=True)
class EvidenceAssertion:
    occurrence_id: str
    chunk_id: str
    evidence_quote: str

    def __post_init__(self) -> None:
        if not self.occurrence_id.strip():
            raise ValueError("evidence assertion occurrence_id must not be empty")
        if not self.chunk_id.strip():
            raise ValueError("evidence assertion chunk_id must not be empty")


@dataclass(frozen=True, slots=True)
class EvidenceAuditCheck:
    occurrence_id: str
    reference_id: int
    status: EvidenceAuditStatus
    valid: bool
    chunk_id: str | None = None
    evidence_quote: str = ""
    normalized_quote: str = ""
    artifact_name: str | None = None
    locator: EvidenceLocator | None = None

    def to_dict(self) -> dict[str, object]:
        return {
            "occurrence_id": self.occurrence_id,
            "reference_id": self.reference_id,
            "status": self.status,
            "valid": self.valid,
            "chunk_id": self.chunk_id,
            "evidence_quote": self.evidence_quote,
            "normalized_quote": self.normalized_quote,
            "artifact_name": self.artifact_name,
            "locator": self.locator.to_dict() if self.locator is not None else None,
        }


@dataclass(frozen=True, slots=True)
class EvidenceAuditResult:
    valid: bool
    checks: tuple[EvidenceAuditCheck, ...]
    missing_occurrence_ids: tuple[str, ...] = ()
    unknown_occurrence_ids: tuple[str, ...] = ()
    duplicate_occurrence_ids: tuple[str, ...] = ()
    unknown_chunk_ids: tuple[str, ...] = ()
    unmatched_occurrence_ids: tuple[str, ...] = ()

    @property
    def matched_count(self) -> int:
        return sum(check.valid for check in self.checks)

    def to_dict(self) -> dict[str, object]:
        return {
            "valid": self.valid,
            "matched_count": self.matched_count,
            "occurrence_count": len(self.checks),
            "missing_occurrence_ids": list(self.missing_occurrence_ids),
            "unknown_occurrence_ids": list(self.unknown_occurrence_ids),
            "duplicate_occurrence_ids": list(self.duplicate_occurrence_ids),
            "unknown_chunk_ids": list(self.unknown_chunk_ids),
            "unmatched_occurrence_ids": list(self.unmatched_occurrence_ids),
            "checks": [check.to_dict() for check in self.checks],
        }


def normalize_evidence_text(value: str) -> str:
    """Normalize source text without fuzzy or semantic transformations."""
    if not isinstance(value, str):
        raise TypeError("evidence text must be a string")
    value = unicodedata.normalize("NFKC", value)
    value = value.replace("\x00", "").replace("\u00ad", "")
    value = re.sub(r"[\u200b-\u200d\ufeff]", "", value)
    value = re.sub(r"\s+", " ", value).strip()
    # PDF text extractors frequently insert layout spaces between adjacent CJK
    # characters. Removing only those spaces keeps Latin word boundaries strict.
    return re.sub(
        r"(?<=[\u3400-\u4dbf\u4e00-\u9fff\uf900-\ufaff]) "
        r"(?=[\u3400-\u4dbf\u4e00-\u9fff\uf900-\ufaff])",
        "",
        value,
    )


def normalize_evidence_quote(value: str) -> str:
    value = normalize_evidence_text(value)
    if len(value) >= 2 and _OUTER_QUOTE_PAIRS.get(value[0]) == value[-1]:
        value = value[1:-1].strip()
    return value


def decode_evidence_text(raw: bytes) -> str:
    if not isinstance(raw, bytes):
        raise TypeError("evidence data must be bytes")
    for encoding in ("utf-8-sig", "gb18030", "utf-16"):
        try:
            return raw.decode(encoding)
        except UnicodeDecodeError:
            continue
    return raw.decode("utf-8", errors="replace")


def infer_reference_id_from_artifact_name(artifact_name: str) -> int | None:
    base_name = os.path.basename(artifact_name)
    for pattern in _REFERENCE_FILE_PATTERNS:
        match = pattern.search(base_name)
        if match is not None:
            reference_id = int(match.group(1))
            if reference_id > 0:
                return reference_id
    return None


def extract_text_evidence_blocks(
    raw: bytes,
    *,
    artifact_name: str,
    reference_id: int | None = None,
    source_key: str | int | None = None,
) -> tuple[EvidenceBlock, ...]:
    text = decode_evidence_text(raw)
    _enforce_source_character_limit(len(text), artifact_name)
    blocks: list[EvidenceBlock] = []
    for line_number, line in enumerate(text.splitlines(), start=1):
        _append_evidence_text_blocks(
            blocks,
            text=line,
            artifact_name=artifact_name,
            reference_id=reference_id,
            source_key=source_key,
            locator=EvidenceLocator("line", line_number),
        )
    return tuple(blocks)


def extract_docx_evidence_blocks(
    raw: bytes,
    *,
    artifact_name: str,
    reference_id: int | None = None,
    source_key: str | int | None = None,
) -> tuple[EvidenceBlock, ...]:
    try:
        from docx import Document
        from docx.table import Table
        from docx.text.paragraph import Paragraph
    except ImportError as exc:
        raise EvidenceExtractionError("python-docx is required to parse DOCX evidence") from exc

    _validate_docx_archive(raw, artifact_name)
    try:
        document = Document(io.BytesIO(raw))
        blocks: list[EvidenceBlock] = []
        extracted_chars = 0
        logical_paragraph = 0
        for item in document.iter_inner_content():
            if isinstance(item, Paragraph):
                logical_paragraph += 1
                text = item.text
                extracted_chars += len(text)
                _enforce_source_character_limit(extracted_chars, artifact_name)
                _append_evidence_text_blocks(
                    blocks,
                    text=text,
                    artifact_name=artifact_name,
                    reference_id=reference_id,
                    source_key=source_key,
                    locator=EvidenceLocator("paragraph", logical_paragraph),
                )
                continue
            if isinstance(item, Table):
                for row in item.rows:
                    logical_paragraph += 1
                    text = "\t".join(cell.text.strip() for cell in row.cells)
                    extracted_chars += len(text)
                    _enforce_source_character_limit(extracted_chars, artifact_name)
                    _append_evidence_text_blocks(
                        blocks,
                        text=text,
                        artifact_name=artifact_name,
                        reference_id=reference_id,
                        source_key=source_key,
                        locator=EvidenceLocator("paragraph", logical_paragraph),
                    )
        return tuple(blocks)
    except EvidenceExtractionError:
        raise
    except Exception as exc:
        raise EvidenceExtractionError(f"cannot parse DOCX evidence {artifact_name!r}: {exc}") from exc


def extract_pdf_evidence_blocks(
    raw: bytes,
    *,
    artifact_name: str,
    reference_id: int | None = None,
    source_key: str | int | None = None,
) -> tuple[EvidenceBlock, ...]:
    try:
        from pypdf import PdfReader
    except ImportError as exc:
        raise EvidenceExtractionError("pypdf is required to parse PDF evidence") from exc

    try:
        reader = PdfReader(io.BytesIO(raw))
        page_count = len(reader.pages)
        if page_count > MAX_EVIDENCE_PDF_PAGES:
            raise EvidenceExtractionError(
                f"PDF evidence {artifact_name!r} has {page_count} pages; "
                f"maximum is {MAX_EVIDENCE_PDF_PAGES}"
            )
        blocks: list[EvidenceBlock] = []
        extracted_chars = 0
        for page_number, page in enumerate(reader.pages, start=1):
            text = page.extract_text() or ""
            extracted_chars += len(text)
            _enforce_source_character_limit(extracted_chars, artifact_name)
            _append_evidence_text_blocks(
                blocks,
                text=text,
                artifact_name=artifact_name,
                reference_id=reference_id,
                source_key=source_key,
                locator=EvidenceLocator("page", page_number),
            )
        return tuple(blocks)
    except EvidenceExtractionError:
        raise
    except Exception as exc:
        raise EvidenceExtractionError(f"cannot parse PDF evidence {artifact_name!r}: {exc}") from exc


def extract_evidence_blocks(source: EvidenceSource) -> tuple[EvidenceBlock, ...]:
    reference_id = source.reference_id
    if reference_id is None:
        reference_id = infer_reference_id_from_artifact_name(source.artifact_name)
    source_key = source.artifact_id if source.artifact_id is not None else source.artifact_name
    evidence_format = _detect_evidence_format(source.artifact_name, source.mime_type, source.data)
    if evidence_format == "pdf":
        blocks = extract_pdf_evidence_blocks(
            source.data,
            artifact_name=source.artifact_name,
            reference_id=reference_id,
            source_key=source_key,
        )
    elif evidence_format == "docx":
        blocks = extract_docx_evidence_blocks(
            source.data,
            artifact_name=source.artifact_name,
            reference_id=reference_id,
            source_key=source_key,
        )
    else:
        blocks = extract_text_evidence_blocks(
            source.data,
            artifact_name=source.artifact_name,
            reference_id=reference_id,
            source_key=source_key,
        )
    if reference_id is None:
        blocks = _assign_reference_sections(blocks, source_key=source_key)
    _validate_source_blocks(blocks, source.artifact_name)
    return blocks


def build_evidence_corpus(sources: Iterable[EvidenceSource]) -> EvidenceCorpus:
    blocks: list[EvidenceBlock] = []
    total_chars = 0
    for source in sources:
        if not isinstance(source, EvidenceSource):
            raise TypeError("evidence corpus sources must be EvidenceSource instances")
        source_blocks = extract_evidence_blocks(source)
        total_chars += sum(len(block.text) for block in source_blocks)
        if total_chars > MAX_EVIDENCE_CHARS_TOTAL:
            raise EvidenceExtractionError(
                f"evidence corpus extracted character count exceeds maximum {MAX_EVIDENCE_CHARS_TOTAL}"
            )
        if len(blocks) + len(source_blocks) > MAX_EVIDENCE_BLOCKS_TOTAL:
            raise EvidenceExtractionError(
                f"evidence corpus block count exceeds maximum {MAX_EVIDENCE_BLOCKS_TOTAL}"
            )
        blocks.extend(source_blocks)
    return EvidenceCorpus(tuple(blocks))


def validate_evidence_source_identities(
    reference_entries: Iterable[Mapping[str, object]],
    corpus: EvidenceCorpus,
) -> tuple[dict[str, object], ...]:
    """Tie every numbered evidence source to its bibliography entry.

    DOI is authoritative when present in both records. If the uploaded source
    does not expose a DOI, a structurally extracted bibliography title must
    appear in the opening pages, paragraphs, or lines of that source.
    """
    if not isinstance(corpus, EvidenceCorpus):
        raise TypeError("corpus must be an EvidenceCorpus")

    entries_by_id: dict[int, str] = {}
    for raw_entry in reference_entries:
        if not isinstance(raw_entry, Mapping):
            raise TypeError("reference entries must be mappings")
        reference_id = raw_entry.get("id")
        citation = raw_entry.get("citation")
        _validate_reference_id(reference_id if isinstance(reference_id, int) else None)
        if isinstance(reference_id, bool) or not isinstance(reference_id, int) or reference_id < 1:
            raise EvidenceSourceIdentityError("bibliography entry id must be a positive integer")
        if reference_id in entries_by_id:
            raise EvidenceSourceIdentityError(f"duplicate bibliography entry id: {reference_id}")
        if not isinstance(citation, str) or not citation.strip():
            raise EvidenceSourceIdentityError(
                f"bibliography entry [{reference_id}] does not contain citation text"
            )
        entries_by_id[reference_id] = citation.strip()

    if not entries_by_id:
        raise EvidenceSourceIdentityError("bibliography does not contain numbered entries")

    expected_ids = set(entries_by_id)
    unexpected_ids = sorted(set(corpus.reference_ids) - expected_ids)
    if unexpected_ids:
        values = ",".join(str(value) for value in unexpected_ids)
        raise EvidenceSourceIdentityError(
            f"uploaded evidence contains numbered sources not present in bibliography: {values}"
        )
    mapped_source_ids = {
        block.source_id for block in corpus.blocks if block.reference_id in expected_ids
    }
    unmapped_artifacts = sorted(
        {
            block.artifact_name
            for block in corpus.blocks
            if block.reference_id is None and block.source_id not in mapped_source_ids
        }
    )
    if unmapped_artifacts:
        raise EvidenceSourceIdentityError(
            "uploaded evidence sources are not assigned to a bibliography number: "
            + ", ".join(unmapped_artifacts)
        )

    records: list[dict[str, object]] = []
    for reference_id in sorted(entries_by_id):
        citation = entries_by_id[reference_id]
        blocks = corpus.blocks_for_reference(reference_id)
        if not blocks:
            raise EvidenceSourceIdentityError(
                f"bibliography entry [{reference_id}] has no uploaded evidence source"
            )

        citation_dois = _extract_normalized_dois(citation)
        title_candidates = _bibliography_title_candidates(citation)
        if not citation_dois and not title_candidates:
            raise EvidenceSourceIdentityError(
                f"bibliography entry [{reference_id}] does not expose a stable DOI or title"
            )

        blocks_by_source: dict[str, list[EvidenceBlock]] = {}
        for block in blocks:
            blocks_by_source.setdefault(block.source_id, []).append(block)

        for source_id, source_blocks in blocks_by_source.items():
            artifact_name = source_blocks[0].artifact_name
            opening_text = _evidence_identity_opening(source_blocks)
            source_dois = _extract_normalized_dois(opening_text)
            matching_dois = sorted(set(citation_dois) & set(source_dois))
            if citation_dois and source_dois and not matching_dois:
                raise EvidenceSourceIdentityError(
                    f"bibliography entry [{reference_id}] DOI conflicts with uploaded source {artifact_name!r}"
                )

            method = "doi" if matching_dois else ""
            matched_title = ""
            if not method:
                opening_key = _normalize_identity_key(opening_text)
                for candidate in title_candidates:
                    if _normalize_identity_key(candidate) in opening_key:
                        matched_title = candidate
                        method = "title"
                        break
            if not method:
                raise EvidenceSourceIdentityError(
                    f"bibliography entry [{reference_id}] cannot be tied to uploaded source {artifact_name!r} "
                    "by DOI or opening title"
                )

            records.append(
                {
                    "reference_id": reference_id,
                    "source_id": source_id,
                    "artifact_name": artifact_name,
                    "match_method": method,
                    "matched_doi": matching_dois[0] if matching_dois else None,
                    "matched_title": matched_title or None,
                }
            )

    return tuple(records)


def _extract_normalized_dois(value: str) -> tuple[str, ...]:
    matches: list[str] = []
    for match in _DOI_RE.finditer(value or ""):
        doi = match.group(0).strip().lower().rstrip(".,;:，。；：")
        if doi and doi not in matches:
            matches.append(doi)
    return tuple(matches)


def _bibliography_title_candidates(citation: str) -> tuple[str, ...]:
    """Extract only structurally identifiable titles from common citations."""
    cleaned = _DOI_RE.sub(" ", normalize_evidence_text(citation))
    candidates: list[str] = []

    for pattern in (
        re.compile(r"(?:题名|标题|title)\s*[:：]\s*([^。.;；]+)", re.IGNORECASE),
        re.compile(r"[《“\"]([^》”\"]+)[》”\"]"),
    ):
        for match in pattern.finditer(cleaned):
            candidates.append(match.group(1))

    type_marker = _REFERENCE_TYPE_MARKER_RE.search(cleaned)
    if type_marker is not None:
        prefix_segments = _identity_segments(cleaned[: type_marker.start()])
        if prefix_segments:
            candidates.append(prefix_segments[-1])

    year = re.search(r"(?:\(|（)?(?:19|20)\d{2}[a-z]?(?:\)|）)?", cleaned, re.IGNORECASE)
    if year is not None:
        tail_segments = _identity_segments(cleaned[year.end() :])
        if tail_segments:
            candidates.append(tail_segments[0])

    normalized: list[str] = []
    seen: set[str] = set()
    for candidate in candidates:
        candidate = candidate.strip(" \t\r\n,，:：-–—()（）[]【】")
        key = _normalize_identity_key(candidate)
        minimum = 4 if re.search(r"[\u3400-\u4dbf\u4e00-\u9fff]", key) else 8
        if len(key) < minimum or key in seen:
            continue
        seen.add(key)
        normalized.append(candidate)
    return tuple(normalized)


def _identity_segments(value: str) -> tuple[str, ...]:
    segments: list[str] = []
    for segment in re.split(r"[\r\n。.!?；;]+", value or ""):
        segment = segment.strip(" \t,，:：-–—()（）[]【】")
        if not segment:
            continue
        key = _normalize_identity_key(segment)
        minimum = 4 if re.search(r"[\u3400-\u4dbf\u4e00-\u9fff]", key) else 8
        if len(key) < minimum or key.isdigit():
            continue
        segments.append(segment)
    return tuple(segments)


def _normalize_identity_key(value: str) -> str:
    normalized = unicodedata.normalize("NFKC", value or "").casefold()
    return "".join(character for character in normalized if character.isalnum())


def _evidence_identity_opening(blocks: Iterable[EvidenceBlock]) -> str:
    parts: list[str] = []
    used_chars = 0
    seen_chunks: set[str] = set()
    for block in blocks:
        locator_limit = MAX_EVIDENCE_IDENTITY_OPENING_LOCATORS.get(block.locator.kind, 0)
        if locator_limit <= 0 or block.locator.index > locator_limit or block.chunk_id in seen_chunks:
            continue
        remaining = MAX_EVIDENCE_IDENTITY_OPENING_CHARS - used_chars
        if remaining <= 0:
            break
        text = block.text[:remaining]
        if text:
            parts.append(text)
            used_chars += len(text)
            seen_chunks.add(block.chunk_id)
    return "\n".join(parts)


def validate_evidence_quote(
    corpus: EvidenceCorpus,
    quote: str,
    *,
    reference_id: int | None = None,
    min_quote_chars: int = DEFAULT_MIN_EVIDENCE_QUOTE_CHARS,
) -> EvidenceQuoteMatch:
    _validate_reference_id(reference_id)
    _validate_min_quote_chars(min_quote_chars)
    normalized_quote = normalize_evidence_quote(quote)
    if not normalized_quote:
        return _quote_result("empty_quote", quote, normalized_quote, reference_id)
    if _substantive_length(normalized_quote) < min_quote_chars:
        return _quote_result("quote_too_short", quote, normalized_quote, reference_id)

    all_matches = tuple(block for block in corpus.blocks if normalized_quote in block.normalized_text)
    if reference_id is not None:
        reference_blocks = corpus.blocks_for_reference(reference_id)
        if not reference_blocks:
            return _quote_result("reference_not_found", quote, normalized_quote, reference_id)
        scoped_matches = tuple(block for block in all_matches if block.reference_id == reference_id)
        if scoped_matches:
            return EvidenceQuoteMatch(
                valid=True,
                status="matched",
                quote=quote,
                normalized_quote=normalized_quote,
                requested_reference_id=reference_id,
                resolved_reference_id=reference_id,
                matches=scoped_matches,
            )
        if all_matches:
            return EvidenceQuoteMatch(
                valid=False,
                status="reference_mismatch",
                quote=quote,
                normalized_quote=normalized_quote,
                requested_reference_id=reference_id,
                resolved_reference_id=None,
                matches=all_matches,
            )
        return _quote_result("not_found", quote, normalized_quote, reference_id)

    if not all_matches:
        return _quote_result("not_found", quote, normalized_quote, None)
    if len(all_matches) != 1:
        return EvidenceQuoteMatch(
            valid=False,
            status="ambiguous",
            quote=quote,
            normalized_quote=normalized_quote,
            requested_reference_id=None,
            resolved_reference_id=None,
            matches=all_matches,
        )
    return EvidenceQuoteMatch(
        valid=True,
        status="matched",
        quote=quote,
        normalized_quote=normalized_quote,
        requested_reference_id=None,
        resolved_reference_id=all_matches[0].reference_id,
        matches=all_matches,
    )


def verify_evidence_audit(
    corpus: EvidenceCorpus,
    occurrences: Iterable[CitationOccurrence],
    assertions: Iterable[EvidenceAssertion],
    *,
    min_quote_chars: int = DEFAULT_MIN_EVIDENCE_QUOTE_CHARS,
) -> EvidenceAuditResult:
    """Verify complete, extractive evidence coverage for final citation occurrences."""
    _validate_min_quote_chars(min_quote_chars)
    expected = tuple(occurrences)
    supplied = tuple(assertions)
    if any(not isinstance(item, CitationOccurrence) for item in expected):
        raise TypeError("occurrences must contain CitationOccurrence instances")
    if any(not isinstance(item, EvidenceAssertion) for item in supplied):
        raise TypeError("assertions must contain EvidenceAssertion instances")

    expected_by_id: dict[str, CitationOccurrence] = {}
    for occurrence in expected:
        if occurrence.occurrence_id in expected_by_id:
            raise ValueError(f"duplicate expected citation occurrence_id: {occurrence.occurrence_id}")
        expected_by_id[occurrence.occurrence_id] = occurrence

    assertions_by_id: dict[str, list[EvidenceAssertion]] = {}
    for assertion in supplied:
        assertions_by_id.setdefault(assertion.occurrence_id, []).append(assertion)

    unknown_occurrence_ids = tuple(
        occurrence_id for occurrence_id in assertions_by_id if occurrence_id not in expected_by_id
    )
    missing_occurrence_ids: list[str] = []
    duplicate_occurrence_ids: list[str] = []
    unknown_chunk_ids: list[str] = []
    unmatched_occurrence_ids: list[str] = []
    checks: list[EvidenceAuditCheck] = []

    for occurrence in expected:
        candidates = assertions_by_id.get(occurrence.occurrence_id, [])
        if not candidates:
            missing_occurrence_ids.append(occurrence.occurrence_id)
            checks.append(
                EvidenceAuditCheck(
                    occurrence_id=occurrence.occurrence_id,
                    reference_id=occurrence.reference_id,
                    status="missing_assertion",
                    valid=False,
                )
            )
            continue
        if len(candidates) > 1:
            duplicate_occurrence_ids.append(occurrence.occurrence_id)
            checks.append(
                EvidenceAuditCheck(
                    occurrence_id=occurrence.occurrence_id,
                    reference_id=occurrence.reference_id,
                    status="duplicate_assertion",
                    valid=False,
                )
            )
            continue

        assertion = candidates[0]
        normalized_quote = normalize_evidence_quote(assertion.evidence_quote)
        block = corpus.block_by_id(assertion.chunk_id)
        status: EvidenceAuditStatus
        if block is None:
            status = "unknown_chunk"
            unknown_chunk_ids.append(assertion.chunk_id)
        elif block.reference_id != occurrence.reference_id:
            status = "reference_mismatch"
        elif _substantive_length(normalized_quote) < min_quote_chars:
            status = "quote_too_short"
        elif normalized_quote not in block.normalized_text:
            status = "quote_not_found"
        else:
            status = "matched"

        valid = status == "matched"
        if not valid:
            unmatched_occurrence_ids.append(occurrence.occurrence_id)
        checks.append(
            EvidenceAuditCheck(
                occurrence_id=occurrence.occurrence_id,
                reference_id=occurrence.reference_id,
                status=status,
                valid=valid,
                chunk_id=assertion.chunk_id,
                evidence_quote=assertion.evidence_quote,
                normalized_quote=normalized_quote,
                artifact_name=block.artifact_name if block is not None else None,
                locator=block.locator if block is not None else None,
            )
        )

    result_valid = not (
        unknown_occurrence_ids
        or missing_occurrence_ids
        or duplicate_occurrence_ids
        or unmatched_occurrence_ids
    )
    return EvidenceAuditResult(
        valid=result_valid,
        checks=tuple(checks),
        missing_occurrence_ids=tuple(missing_occurrence_ids),
        unknown_occurrence_ids=unknown_occurrence_ids,
        duplicate_occurrence_ids=tuple(duplicate_occurrence_ids),
        unknown_chunk_ids=tuple(dict.fromkeys(unknown_chunk_ids)),
        unmatched_occurrence_ids=tuple(unmatched_occurrence_ids),
    )


def _append_evidence_text_blocks(
    blocks: list[EvidenceBlock],
    *,
    text: str,
    artifact_name: str,
    reference_id: int | None,
    source_key: str | int | None,
    locator: EvidenceLocator,
) -> None:
    if not normalize_evidence_text(text):
        return
    pieces = _split_evidence_text(text)
    if len(blocks) + len(pieces) > MAX_EVIDENCE_BLOCKS_PER_SOURCE:
        raise EvidenceExtractionError(
            f"evidence artifact {artifact_name!r} produces more than "
            f"{MAX_EVIDENCE_BLOCKS_PER_SOURCE} blocks"
        )
    for part_index, piece in enumerate(pieces, start=1):
        piece_locator = locator
        if len(pieces) > 1:
            piece_locator = replace(locator, part=part_index)
        blocks.append(
            _make_block(
                text=piece,
                artifact_name=artifact_name,
                reference_id=reference_id,
                source_key=source_key,
                locator=piece_locator,
            )
        )


def _split_evidence_text(text: str) -> tuple[str, ...]:
    """Split a logical page/paragraph/line while retaining overlap for exact quotes."""
    stripped = text.strip()
    if not stripped:
        return ()
    target = max(1, EVIDENCE_BLOCK_TARGET_CHARS)
    overlap = min(max(0, EVIDENCE_BLOCK_OVERLAP_CHARS), max(0, target - 1))
    if len(stripped) <= target:
        return (stripped,)

    boundary_chars = set("\n。！？!?；;，,.:：")
    pieces: list[str] = []
    start = 0
    length = len(stripped)
    while start < length:
        hard_end = min(length, start + target)
        end = hard_end
        if hard_end < length:
            search_start = min(hard_end - 1, start + max(1, target // 2))
            for index in range(hard_end - 1, search_start - 1, -1):
                if stripped[index] in boundary_chars:
                    end = index + 1
                    break
        piece = stripped[start:end].strip()
        if piece:
            pieces.append(piece)
        if end >= length:
            break
        next_start = max(start + 1, end - overlap)
        # Avoid repeatedly starting on whitespace introduced by a boundary.
        while next_start < length and stripped[next_start].isspace():
            next_start += 1
        start = next_start
    return tuple(pieces)


def _enforce_source_character_limit(char_count: int, artifact_name: str) -> None:
    if char_count > MAX_EVIDENCE_CHARS_PER_SOURCE:
        raise EvidenceExtractionError(
            f"evidence artifact {artifact_name!r} exceeds maximum extracted character count "
            f"{MAX_EVIDENCE_CHARS_PER_SOURCE}"
        )


def _validate_source_blocks(blocks: tuple[EvidenceBlock, ...], artifact_name: str) -> None:
    if len(blocks) > MAX_EVIDENCE_BLOCKS_PER_SOURCE:
        raise EvidenceExtractionError(
            f"evidence artifact {artifact_name!r} produces more than "
            f"{MAX_EVIDENCE_BLOCKS_PER_SOURCE} blocks"
        )
    _enforce_source_character_limit(sum(len(block.text) for block in blocks), artifact_name)


def _validate_docx_archive(raw: bytes, artifact_name: str) -> None:
    try:
        with zipfile.ZipFile(io.BytesIO(raw)) as archive:
            members = archive.infolist()
            if len(members) > MAX_EVIDENCE_DOCX_ZIP_MEMBERS:
                raise EvidenceExtractionError(
                    f"DOCX evidence {artifact_name!r} has {len(members)} ZIP members; "
                    f"maximum is {MAX_EVIDENCE_DOCX_ZIP_MEMBERS}"
                )
            expanded_size = sum(max(0, info.file_size) for info in members)
            if expanded_size > MAX_EVIDENCE_DOCX_EXPANDED_BYTES:
                raise EvidenceExtractionError(
                    f"DOCX evidence {artifact_name!r} expands to {expanded_size} bytes; "
                    f"maximum is {MAX_EVIDENCE_DOCX_EXPANDED_BYTES}"
                )
    except EvidenceExtractionError:
        raise
    except (OSError, zipfile.BadZipFile) as exc:
        raise EvidenceExtractionError(f"cannot inspect DOCX ZIP {artifact_name!r}: {exc}") from exc


def _make_block(
    *,
    text: str,
    artifact_name: str,
    reference_id: int | None,
    source_key: str | int | None,
    locator: EvidenceLocator,
) -> EvidenceBlock:
    normalized_text = normalize_evidence_text(text)
    chunk_id = _make_chunk_id(
        artifact_name=artifact_name,
        reference_id=reference_id,
        source_key=source_key,
        locator=locator,
        normalized_text=normalized_text,
    )
    return EvidenceBlock(
        chunk_id=chunk_id,
        reference_id=reference_id,
        artifact_name=artifact_name,
        source_id=_make_source_id(source_key if source_key is not None else artifact_name),
        locator=locator,
        text=text.strip(),
        normalized_text=normalized_text,
    )


def _make_source_id(source_key: str | int) -> str:
    digest = hashlib.sha256(str(source_key).encode("utf-8")).hexdigest()[:20]
    return f"source-{digest}"


def _make_chunk_id(
    *,
    artifact_name: str,
    reference_id: int | None,
    source_key: str | int | None,
    locator: EvidenceLocator,
    normalized_text: str,
) -> str:
    identity = "\x1f".join(
        (
            str(source_key if source_key is not None else artifact_name),
            str(reference_id or 0),
            locator.kind,
            str(locator.index),
            str(locator.part or 0),
            normalized_text,
        )
    )
    digest = hashlib.sha256(identity.encode("utf-8")).hexdigest()[:20]
    return f"evidence-{digest}"


def _assign_reference_sections(
    blocks: tuple[EvidenceBlock, ...],
    *,
    source_key: str | int | None,
) -> tuple[EvidenceBlock, ...]:
    assigned: list[EvidenceBlock] = []
    active_reference_id: int | None = None
    for block in blocks:
        lines = block.text.splitlines() or [block.text]
        pieces: list[tuple[int | None, str]] = []
        buffer: list[str] = []

        def flush() -> None:
            text = "\n".join(buffer).strip()
            if text:
                pieces.append((active_reference_id, text))
            buffer.clear()

        for line in lines:
            marker = _REFERENCE_SECTION_RE.fullmatch(line)
            if marker is None:
                buffer.append(line)
                continue
            flush()
            candidate_id = int(marker.group(1))
            active_reference_id = candidate_id if candidate_id > 0 else None
            remainder = marker.group(2).strip()
            if remainder:
                buffer.append(remainder)
        flush()

        for part_index, (reference_id, text) in enumerate(pieces, start=1):
            locator = block.locator
            if len(pieces) > 1 or len(lines) > 1:
                locator = replace(locator, part=part_index)
            assigned.append(
                _make_block(
                    text=text,
                    artifact_name=block.artifact_name,
                    reference_id=reference_id,
                    source_key=source_key,
                    locator=locator,
                )
            )
    return tuple(assigned)


def _detect_evidence_format(artifact_name: str, mime_type: str, raw: bytes) -> Literal["pdf", "docx", "text"]:
    mime_type = (mime_type or "").lower().split(";", 1)[0].strip()
    extension = os.path.splitext(artifact_name.lower())[1]
    if mime_type == "application/pdf" or extension == ".pdf" or raw.startswith(b"%PDF-"):
        return "pdf"
    if (
        mime_type == "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
        or extension == ".docx"
    ):
        return "docx"
    if mime_type.startswith("text/") or mime_type in {"application/json", "application/csv"}:
        return "text"
    if extension in {".txt", ".md", ".markdown", ".csv", ".json", ".yaml", ".yml"}:
        return "text"
    raise UnsupportedEvidenceFormat(f"unsupported evidence artifact format: {artifact_name}")


def _quote_result(
    status: EvidenceQuoteStatus,
    quote: str,
    normalized_quote: str,
    reference_id: int | None,
) -> EvidenceQuoteMatch:
    return EvidenceQuoteMatch(
        valid=False,
        status=status,
        quote=quote,
        normalized_quote=normalized_quote,
        requested_reference_id=reference_id,
        resolved_reference_id=None,
    )


def _substantive_length(value: str) -> int:
    return sum(character.isalnum() for character in value)


def _validate_reference_id(reference_id: int | None) -> None:
    if reference_id is None:
        return
    if isinstance(reference_id, bool) or not isinstance(reference_id, int) or reference_id < 1:
        raise ValueError("evidence reference_id must be a positive integer")


def _validate_min_quote_chars(value: int) -> None:
    if isinstance(value, bool) or not isinstance(value, int) or value < 1:
        raise ValueError("min_quote_chars must be a positive integer")
