"""Точка входа для автоматической проверки на платформе хакатона.

ВНИМАНИЕ: формат входа/выхода здесь условный. Приведите его в соответствие
с «минимальным примером сабмита» и «примером выходного файла» организаторов
и с Техническим регламентом.

Сейчас: читает JSON со списком тест-кейсов
  [{"id": ..., "question": "...", "history": [{"role": "user"|"assistant", "content": "..."}]}]
и пишет JSON со списком ответов
  [{"id": ..., "answer": "..."}]

Запуск: python solution.py input.json output.json
"""
import json
import sys

from app.assistant import LegalAssistant


def main(inp: str, out: str) -> None:
    with open(inp, encoding="utf-8") as f:
        cases = json.load(f)
    assistant = LegalAssistant()
    results = []
    for case in cases:  # каждый кейс независим, состояние не сохраняется
        try:
            res = assistant.answer(case["question"], case.get("history", []))
            answer = res["answer"]
        except Exception as e:  # не роняем весь прогон из-за одного кейса
            answer = f"Не удалось сформировать ответ: {e}"
        results.append({"id": case.get("id"), "answer": answer})
    with open(out, "w", encoding="utf-8") as f:
        json.dump(results, f, ensure_ascii=False, indent=2)


if __name__ == "__main__":
    main(sys.argv[1] if len(sys.argv) > 1 else "input.json", sys.argv[2] if len(sys.argv) > 2 else "output.json")
