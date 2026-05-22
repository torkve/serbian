#!/usr/bin/env bash
# Download a whisper.cpp ggml model into ./models/.
# Usage: WHISPER_MODEL=medium ./scripts/fetch-whisper-model.sh
#
# Recognized model names mirror the upstream download-ggml-model.sh script:
#   tiny tiny.en base base.en small small.en medium medium.en large-v1
#   large-v2 large-v3 large-v3-turbo
# (and the -q5_1/-q8_0 quantized variants).

set -euo pipefail

MODEL="${WHISPER_MODEL:-medium}"
DEST_DIR="${WHISPER_MODELS_DIR:-models}"
DEST_FILE="${DEST_DIR}/ggml-${MODEL}.bin"

# Canonical URL used by whisper.cpp itself.
BASE_URL="https://huggingface.co/ggerganov/whisper.cpp/resolve/main"
URL="${BASE_URL}/ggml-${MODEL}.bin"

mkdir -p "${DEST_DIR}"

if [[ -s "${DEST_FILE}" ]]; then
    size=$(stat -c '%s' "${DEST_FILE}" 2>/dev/null || stat -f '%z' "${DEST_FILE}")
    echo "Model already present: ${DEST_FILE} (${size} bytes). Skipping download."
    echo "  Delete the file to force a re-download."
    exit 0
fi

if ! command -v curl >/dev/null 2>&1; then
    echo "error: curl is required" >&2
    exit 1
fi

# Rough size hints so the user knows what they're committing to.
case "${MODEL}" in
    tiny*|tiny.en*)    sz="~75 MB" ;;
    base*|base.en*)    sz="~145 MB" ;;
    small*|small.en*)  sz="~470 MB" ;;
    medium*|medium.en*) sz="~1.5 GB" ;;
    large-v3-turbo*)   sz="~1.6 GB" ;;
    large*)            sz="~3 GB" ;;
    *)                 sz="unknown size" ;;
esac

echo "Fetching whisper model '${MODEL}' (${sz}) -> ${DEST_FILE}"
echo "  source: ${URL}"

tmp="${DEST_FILE}.part"
trap 'rm -f "${tmp}"' EXIT

curl --fail --location --progress-bar -o "${tmp}" "${URL}"
mv "${tmp}" "${DEST_FILE}"
trap - EXIT

final_size=$(stat -c '%s' "${DEST_FILE}" 2>/dev/null || stat -f '%z' "${DEST_FILE}")
echo "Done: ${DEST_FILE} (${final_size} bytes)"
