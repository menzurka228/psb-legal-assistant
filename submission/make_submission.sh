#!/usr/bin/env bash
# Собирает submission/submission.zip для платформы: solution.py + requirements.txt.
# База знаний и вопросы платформа подкладывает сама (data/kb.zip, data/questions.csv).
set -euo pipefail
cd "$(dirname "$0")"
rm -f submission.zip
zip -q submission.zip solution.py requirements.txt
unzip -l submission.zip
