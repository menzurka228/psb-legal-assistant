"""Настройки приложения из переменных окружения."""
import os
from pathlib import Path

_HERE = Path(__file__).resolve().parent
# В ZIP-сабмите data/ лежит рядом с solution.py, в репозитории — в корне проекта
_DEFAULT_KB = next((p for p in (_HERE.parent / "data" / "kb", _HERE.parent.parent / "data" / "kb") if p.exists()), _HERE.parent / "data" / "kb")
KB_DIR = Path(os.getenv("KB_DIR", _DEFAULT_KB))

# OpenAI-совместимый API (на платформе хакатона — DeepSeek V4.1 Flash через API организаторов)
LLM_BASE_URL = os.getenv("LLM_BASE_URL", "")
LLM_API_KEY = os.getenv("LLM_API_KEY", "")
LLM_MODEL = os.getenv("LLM_MODEL", "deepseek-v4.1-flash")
LLM_TEMPERATURE = float(os.getenv("LLM_TEMPERATURE", "0.1"))

TOP_K = int(os.getenv("TOP_K", "8"))
# Минимальный BM25-скор лучшего фрагмента, ниже которого считаем, что в базе ничего нет
MIN_SCORE = float(os.getenv("MIN_SCORE", "1.0"))
