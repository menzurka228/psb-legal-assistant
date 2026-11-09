#!/usr/bin/env bash
# Собирает ZIP для сабмита на платформу (solution.py + код + индекс + база знаний)
set -euo pipefail
cd "$(dirname "$0")/.."
rm -f submission.zip
zip -r submission.zip solution.py app requirements.txt data index -x '*/__pycache__/*' '*.DS_Store'
echo "Готово: submission.zip"
