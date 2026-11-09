"""Оценка поиска по законам (для ML / Retrieval).

Для каждого вопроса из data/eval/dataset.csv, у которого в `sources` есть законы, проверяет,
попали ли нужные документы в топ-K найденных фрагментов `Retriever` из solution.py.
Документы компании в промпт идут целиком, поэтому здесь оцениваются только законы.

Метрики:
  hit@K    — хотя бы один нужный закон в топ-K;
  recall@K — доля нужных законов, найденных в топ-K;
  MRR      — 1 / позиция первого нужного фрагмента.

Запуск:
  python submission/eval_retrieval.py                       # офлайн: только BM25
  LLM_BASE_URL=... LLM_API_KEY=... python submission/eval_retrieval.py --api   # + эмбеддинги и реранкер
"""

from __future__ import annotations

import argparse
import csv
import sys
from pathlib import Path

import httpx

sys.path.insert(0, str(Path(__file__).resolve().parent))
import solution as s  # noqa: E402

ROOT = Path(__file__).resolve().parent.parent


def main() -> None:
    ap = argparse.ArgumentParser()
    ap.add_argument("--dataset", type=Path, default=ROOT / "data/eval/dataset.csv")
    ap.add_argument("--kb", type=Path, default=ROOT / "data/kb")
    ap.add_argument("--k", type=int, nargs="+", default=[3, 5, 8])
    ap.add_argument("--candidates", type=int, default=s.LAW_CANDIDATES)
    ap.add_argument("--api", action="store_true", help="использовать эмбеддинги и реранкер через API")
    ap.add_argument("--report", type=Path, default=ROOT / "submission/retrieval_report.csv")
    args = ap.parse_args()

    assistant = s.Assistant(s.load_documents(args.kb))
    retriever = assistant.law_retriever
    client = httpx.Client(timeout=120) if args.api else None
    if client:
        retriever.prepare_embeddings(client)

    rows = list(csv.DictReader(args.dataset.open(encoding="utf-8")))
    max_k = max(args.k)
    results = []
    for r in rows:
        expected = {p.split("|")[0].strip() for p in r["sources"].split(";") if p.strip().startswith("законы__")}
        if not expected:
            continue
        msgs = s.parse_dialog(r["question"])
        query = " ".join(m["content"] for m in msgs if m["role"] == "user")
        found = [c.doc.name for c in retriever.search(client, query, args.candidates, max_k)]
        first = next((i + 1 for i, d in enumerate(found) if d in expected), None)
        res = {"id": r["id"], "category": r["category"], "question": r["question"][:90],
               "expected": "; ".join(sorted(expected)), "found_top": "; ".join(dict.fromkeys(found[:5])),
               "rr": 1 / first if first else 0.0}
        for k in args.k:
            top = set(found[:k])
            res[f"hit@{k}"] = int(bool(top & expected))
            res[f"recall@{k}"] = len(top & expected) / len(expected)
        results.append(res)

    n = len(results)
    print(f"Режим: {'гибрид (API)' if client else 'только BM25'}; вопросов с законами: {n}")
    for k in args.k:
        hit = sum(x[f"hit@{k}"] for x in results) / n
        rec = sum(x[f"recall@{k}"] for x in results) / n
        print(f"  hit@{k}: {hit:.2f}   recall@{k}: {rec:.2f}")
    print(f"  MRR: {sum(x['rr'] for x in results) / n:.2f}")

    misses = [x for x in results if not x[f"hit@{max_k}"]]
    if misses:
        print(f"\nПромахи (нужный закон не попал в топ-{max_k}):")
        for x in misses:
            print(f"  {x['id']}: {x['question']}\n     нужно: {x['expected']}\n     нашлось: {x['found_top']}")

    with args.report.open("w", encoding="utf-8", newline="") as f:
        w = csv.DictWriter(f, fieldnames=list(results[0]), quoting=csv.QUOTE_ALL)
        w.writeheader()
        w.writerows(results)
    print(f"\nОтчёт: {args.report}")


if __name__ == "__main__":
    main()
