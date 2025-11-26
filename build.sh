#!/usr/bin/env bash
# build.sh - Build the EpGo Docker image (multi-arch aware).
#
# Usage examples:
#   ./build.sh                                # prompts for REF and tag values, loads native arch locally
#   ./build.sh -r v1.3-RC -t dev              # passes REF and tag via flags
#   ./build.sh --ref main --tag qa --push     # build and push multi-arch (amd64/arm64)
#   ./build.sh -r main -t qa --push --latest  # also push the "latest" tag
#   ./build.sh -r main -t qa --push --develop # also push the "develop" tag
#
# Flags:
#   -r, --ref    Build argument for REF (required; prompted if omitted)
#   -t, --tag    Image tag to apply to nillivanilli0815/epgo (required; prompted if omitted)
#   -p, --push   Build and push a multi-arch image (amd64, arm64) to the registry
#   -l, --latest Also tag the image as "latest" (use with --push)
#   -d, --develop Also tag the image as "develop" (use with --push)
#   -h, --help   Show this help message

set -euo pipefail

IMAGE_NAME="nillivanilli0815/epgo"
REF=""
TAG=""
PUSH=false
PUSH_LATEST=false
PUSH_DEVELOP=false
PLATFORMS="linux/amd64,linux/arm64"
BUILDER_NAME="epgo-builder"

print_help() {
  sed -n '1,28p' "$0"
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
    -p|--push)
      PUSH=true
      shift 1
      ;;
    -l|--latest)
      PUSH_LATEST=true
      shift 1
      ;;
    -d|--develop)
      PUSH_DEVELOP=true
      shift 1
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

TAG_ARGS=("-t" "${IMAGE_NAME}:${TAG}")
TAG_LIST=("${IMAGE_NAME}:${TAG}")
if [[ "$PUSH_LATEST" == true ]]; then
  TAG_ARGS+=("-t" "${IMAGE_NAME}:latest")
  TAG_LIST+=("${IMAGE_NAME}:latest")
fi
if [[ "$PUSH_DEVELOP" == true ]]; then
  TAG_ARGS+=("-t" "${IMAGE_NAME}:develop")
  TAG_LIST+=("${IMAGE_NAME}:develop")
fi
TAG_STRING=$(IFS=", "; echo "${TAG_LIST[*]}")

if ! docker buildx inspect "$BUILDER_NAME" >/dev/null 2>&1; then
  docker buildx create --name "$BUILDER_NAME" --use
else
  docker buildx use "$BUILDER_NAME"
fi

if [[ "$PUSH" == true ]]; then
  echo "Building and pushing multi-arch image(s) ${TAG_STRING} for ${PLATFORMS} with REF=${REF}..."
  docker buildx build \
    --platform "$PLATFORMS" \
    --build-arg REF="$REF" \
    --no-cache \
    "${TAG_ARGS[@]}" \
    --push \
    .
else
  # Load a single-architecture image locally; buildx cannot load a multi-arch manifest into the local daemon.
  NATIVE_PLATFORM=$(docker info --format '{{.OSType}}/{{.Architecture}}')
  echo "Building native image(s) ${TAG_STRING} for ${NATIVE_PLATFORM} with REF=${REF} (use --push for multi-arch)..."
  docker buildx build \
    --platform "$NATIVE_PLATFORM" \
    --build-arg REF="$REF" \
    --no-cache \
    "${TAG_ARGS[@]}" \
    --load \
    .
fi

echo "Image build completed."
