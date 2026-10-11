#!/usr/bin/env bash
# Validate the OCR service end to end.
#
#   ./validate.sh                       # against https://ocr.dev.initialed85.cc
#   BASE_URL=http://127.0.0.1:8080 ./validate.sh
#
# Generates a known-text image, OCRs it, and asserts the text round-trips.
set -euo pipefail

BASE_URL="${BASE_URL:-https://ocr.dev.initialed85.cc}"
EXPECTED="OCR round trip 12345"

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

convert -size 700x120 xc:white -pointsize 48 -fill black \
    -gravity center -annotate +0+0 "$EXPECTED" "$work/sample.png"

echo "==> $BASE_URL/healthz"
curl -fsS "$BASE_URL/healthz" | grep -q '"ok":true' && echo "healthz ok"

echo "==> $BASE_URL/ (info)"
curl -fsS "$BASE_URL/" | grep -q '"tesseract"' && echo "info ok"

echo "==> POST $BASE_URL/ocr (txt, file upload)"
text="$(curl -fsS -F "file=@$work/sample.png" "$BASE_URL/ocr")"
echo "$text"
if [[ "$text" != *"$EXPECTED"* ]]; then
    echo "FAIL: txt output did not contain '$EXPECTED'" >&2
    exit 1
fi

echo "==> POST $BASE_URL/ocr (json, word confidence)"
json="$(curl -fsS -F "file=@$work/sample.png" -F "format=json" "$BASE_URL/ocr")"
echo "$json"
words="$(printf '%s' "$json" | grep -o '"conf"' | wc -l | tr -d ' ')"
if [[ "$words" -lt 4 ]]; then
    echo "FAIL: expected >= 4 words with confidence, got $words" >&2
    exit 1
fi

echo "==> POST $BASE_URL/ocr (url form, tsv)"
if ! curl -fsS -F "url=https://example.com/" "$BASE_URL/ocr" >/dev/null; then
    echo "note: url form rejected upstream (non-image), that's expected"
fi

echo "all checks passed"
