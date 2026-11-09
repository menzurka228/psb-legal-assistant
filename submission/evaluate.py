"""Локальная оценка сабмита на демо-вопросах.

1. Собирает временную папку как на платформе: questions.csv + kb.zip (из data/kb).
2. Запускает solution.py → submission.csv.
3. LLM-судья сравнивает каждый ответ с эталоном (0 — неверно, 1 — частично, 2 — верно).
4. Печатает таблицу и сохраняет eval_report.csv.

Запуск (из корня репозитория, ключ берётся из окружения):
    LLM_BASE_URL=... LLM_API_KEY=... python submission/evaluate.py
    python submission/evaluate.py --skip-run     # только оценить уже готовый submission.csv
"""

from __future__ import annotations

import argparse
import csv
import json
import os
import re
import shutil
import subprocess
import sys
import tempfile
import zipfile
from pathlib import Path

import httpx

ROOT = Path(__file__).resolve().parent.parent
SOLUTION = Path(__file__).resolve().parent / "solution.py"

JUDGE_PROMPT = """Ты — строгий эксперт-юрист, оценивающий ответ ИИ-ассистента по эталону.
Вопрос (может быть диалогом):
{question}

Эталонный ответ:
{reference}

Ответ ассистента:
{answer}

Оцени по шкале:
2 — по существу совпадает с эталоном: те же выводы, цифры, сроки и ссылки (формулировки могут отличаться), нет ложных утверждений;
1 — частично верно: главный вывод верен, но упущены важные детали или есть неточность;
0 — неверно: другой вывод, выдуманные нормы/цифры, отказ при наличии ответа или ответ вместо обоснованного отказа.
Верни JSON: {{"score": 0|1|2, "reason": "кратко, что не так"}}"""


def read_csv(path: Path, key: str) -> dict[str, str]:
    with path.open(encoding="utf-8-sig", newline="") as f:
        return {r["id"]: r[key] for r in csv.DictReader(f)}


def run_solution(questions: Path, kb_dir: Path, out: Path) -> None:
    with tempfile.TemporaryDirectory() as tmp:
        data = Path(tmp) / "data"
        data.mkdir()
        shutil.copy(questions, data / "questions.csv")
        with zipfile.ZipFile(data / "kb.zip", "w", zipfile.ZIP_DEFLATED) as z:
            for p in sorted(kb_dir.rglob("*")):
                if p.is_file() and not p.name.startswith("."):
                    z.write(p, p.relative_to(kb_dir))
        env = {**os.environ, "DATA_DIR": str(data), "OUTPUT": str(out)}
        subprocess.run([sys.executable, str(SOLUTION)], env=env, check=True, cwd=tmp)


def judge(client: httpx.Client, question: str, reference: str, answer: str) -> tuple[int, str]:
    base = (os.getenv("LLM_BASE_URL") or os.getenv("BASE_URL") or "").rstrip("/")
    key = os.getenv("LLM_API_KEY") or os.getenv("OPENAI_API_KEY") or ""
    model = os.getenv("JUDGE_MODEL") or os.getenv("LLM_MODEL") or "deepseek/deepseek-v4.1-flash"
    resp = client.post(
        base + "/chat/completions",
        headers={"Authorization": "Bearer " + key},
        json={
            "model": model,
            "temperature": 0,
            "messages": [{"role": "user", "content": JUDGE_PROMPT.format(question=question, reference=reference, answer=answer)}],
        },
    )
    resp.raise_for_status()
    text = resp.json()["choices"][0]["message"]["content"]
    m = re.search(r"\{.*\}", text, re.DOTALL)
    try:
        data = json.loads(m.group(0)) if m else {}
        return int(data.get("score", 0)), str(data.get("reason", ""))
    except (ValueError, json.JSONDecodeError):
        return 0, f"не удалось разобрать ответ судьи: {text[:100]}"


def main() -> None:
    ap = argparse.ArgumentParser()
    ap.add_argument("--questions", type=Path, default=ROOT / "data/demo/questions.csv")
    ap.add_argument("--reference", type=Path, default=ROOT / "data/demo/reference.csv")
    ap.add_argument("--kb", type=Path, default=ROOT / "data/kb")
    ap.add_argument("--out", type=Path, default=ROOT / "submission/submission.csv")
    ap.add_argument("--report", type=Path, default=ROOT / "submission/eval_report.csv")
    ap.add_argument("--skip-run", action="store_true", help="не запускать solution.py, оценить готовый --out")
    args = ap.parse_args()

    if not args.skip_run:
        run_solution(args.questions, args.kb, args.out)

    questions = read_csv(args.questions, "question")
    reference = read_csv(args.reference, "answer")
    answers = read_csv(args.out, "answer")

    rows, total = [], 0
    with httpx.Client(timeout=120) as client:
        for qid, question in questions.items():
            score, reason = judge(client, question, reference.get(qid, ""), answers.get(qid, ""))
            total += score
            rows.append({"id": qid, "score": score, "reason": reason, "question": question,
                         "answer": answers.get(qid, ""), "reference": reference.get(qid, "")})
            print(f"{qid:>4}  {score}  {reason[:110]}")

    with args.report.open("w", encoding="utf-8", newline="") as f:
        w = csv.DictWriter(f, fieldnames=list(rows[0]), quoting=csv.QUOTE_ALL)
        w.writeheader()
        w.writerows(rows)
    print(f"\nИтого: {total}/{2 * len(rows)} ({100 * total / (2 * len(rows)):.0f}%). Отчёт: {args.report}")


if __name__ == "__main__":
    main()
