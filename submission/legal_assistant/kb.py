"""База знаний: загрузка документов, нарезка на положения и поиск (BM25).

Это стартовая версия. Дальше планируется: гибридный поиск (BAAI/bge-m3 + BM25)
и реранкинг (Qwen3 Reranker 8B).
"""
from __future__ import annotations

import re
from dataclasses import dataclass, asdict
from pathlib import Path

from rank_bm25 import BM25Okapi

from . import config

TOKEN_RE = re.compile(r"[а-яёa-z0-9]+", re.IGNORECASE)
# Крупные структурные единицы (родитель для пунктов) и пункты внутри статьи
SECTION_RE = re.compile(r"^\s*(Статья\s+\d+[\.\d]*|Глава\s+\d+|Раздел\s+[\dIVX]+)", re.IGNORECASE)
POINT_RE = re.compile(r"^\s*(\d+(\.\d+)*\.\s|п\.\s*\d+)", re.IGNORECASE)


def tokenize(text: str) -> list[str]:
    # Простейшая нормализация: нижний регистр + обрезка окончаний до 5 символов
    return [t.lower()[:5] for t in TOKEN_RE.findall(text)]


@dataclass
class Chunk:
    id: str
    doc: str
    title: str  # заголовок положения (статья/пункт)
    text: str

    def to_dict(self) -> dict:
        return asdict(self)


def read_document(path: Path) -> str:
    if path.suffix.lower() in {".txt", ".md"}:
        return path.read_text(encoding="utf-8", errors="ignore")
    if path.suffix.lower() == ".docx":
        import docx  # python-docx

        return "\n".join(p.text for p in docx.Document(str(path)).paragraphs)
    if path.suffix.lower() == ".pdf":
        from pypdf import PdfReader

        return "\n".join(page.extract_text() or "" for page in PdfReader(str(path)).pages)
    return ""


def split_into_chunks(doc_name: str, text: str, max_chars: int = 1500) -> list[Chunk]:
    chunks: list[Chunk] = []
    title, buf = "Начало документа", []

    def flush():
        nonlocal buf
        body = "\n".join(buf).strip()
        if body:
            for i in range(0, len(body), max_chars):
                chunks.append(
                    Chunk(id=f"{doc_name}#{len(chunks)}", doc=doc_name, title=title, text=body[i : i + max_chars])
                )

    section = ""  # текущая статья/глава — добавляется к заголовку пункта
    for line in text.splitlines():
        trimmed = line.strip()
        if SECTION_RE.match(line):
            flush()
            section = trimmed[:120]
            title, buf = section, []
        elif POINT_RE.match(line):
            flush()
            point = trimmed[:40] if trimmed.lower().startswith("п.") else "п. " + trimmed.split()[0].rstrip(".")
            title, buf = (f"{section} › {point}" if section else point), []
        buf.append(line)
    flush()
    return chunks


class KnowledgeBase:
    def __init__(self, kb_dir: Path = config.KB_DIR):
        self.kb_dir = Path(kb_dir)
        self.documents: dict[str, str] = {}
        self.chunks: list[Chunk] = []
        self._bm25: BM25Okapi | None = None
        self.load()

    def load(self) -> None:
        for path in sorted(self.kb_dir.rglob("*")):
            if path.is_file() and not path.name.startswith("."):
                text = read_document(path)
                if text.strip():
                    name = str(path.relative_to(self.kb_dir))
                    self.documents[name] = text
                    self.chunks.extend(split_into_chunks(name, text))
        if self.chunks:
            self._bm25 = BM25Okapi([tokenize(c.title + " " + c.text) for c in self.chunks])

    def search(self, query: str, top_k: int = config.TOP_K) -> list[tuple[Chunk, float]]:
        if not self._bm25:
            return []
        scores = self._bm25.get_scores(tokenize(query))
        ranked = sorted(range(len(scores)), key=lambda i: scores[i], reverse=True)[:top_k]
        return [(self.chunks[i], float(scores[i])) for i in ranked if scores[i] > 0]
