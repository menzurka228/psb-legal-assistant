#!/usr/bin/env bash
# Собирает submission.zip для автоматической проверки: solution.py + legal_assistant + база знаний
set -euo pipefail
cd "$(dirname "$0")"
rm -rf build submission.zip && mkdir build
cp -R solution.py legal_assistant requirements.txt build/
cp -R ../data build/data
(cd build && zip -qr ../submission.zip . -x '*/__pycache__/*' '*.DS_Store')
rm -rf build
echo "Готово: submission/submission.zip"
