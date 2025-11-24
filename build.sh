#!/usr/bin/env bash
# build.sh - Build the EpGo Docker image.
#
# Usage examples:
#   ./build.sh                     # prompts for REF and tag values
#   ./build.sh -r v1.3-RC -t dev   # passes REF and tag via flags
#   ./build.sh --ref main --tag qa # long-form flags
#
# Flags:
#   -r, --ref    Build argument for REF (required; prompted if omitted)
#   -t, --tag    Image tag to apply to nillivanilli0815/epgo (required; prompted if omitted)
#   -h, --help   Show this help message

set -euo pipefail

IMAGE_NAME="nillivanilli0815/epgo"
REF=""
TAG=""

print_help() {
  sed -n '1,17p' "$0"
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    -r|--ref)
      REF="${2:-}"
      shift 2
      ;;
    -t|--tag)
      TAG="${2:-}"
      shift 2
      ;;
    -h|--help)
      print_help
      exit 0
      ;;
    *)
      echo "Unknown option: $1" >&2
      print_help
      exit 1
      ;;
  esac
done

if [[ -z "$REF" ]]; then
  read -r -p "Enter REF build argument (e.g., v1.3-RC): " REF
fi

if [[ -z "$TAG" ]]; then
  read -r -p "Enter image tag to use for ${IMAGE_NAME} (e.g., develop): " TAG
fi

if [[ -z "$REF" || -z "$TAG" ]]; then
  echo "Both REF and tag are required." >&2
  exit 1
fi

echo "Building image ${IMAGE_NAME}:${TAG} with REF=${REF}..."
docker build --no-cache --build-arg REF="$REF" -t "${IMAGE_NAME}:${TAG}" .
