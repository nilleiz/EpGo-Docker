# ---------- Stage 1: Builder ----------
FROM --platform=$BUILDPLATFORM golang:1.22-alpine AS builder

ARG TARGETOS
ARG TARGETARCH

RUN apk add --no-cache git ca-certificates
WORKDIR /src

# Build args so you can choose branch/tag/commit at build time
ARG REPO=https://github.com/nilleiz/EpGo-Docker.git
ARG REF=master

# Clone the repo at the requested ref (branch/tag/commit), shallow for speed
RUN git clone --depth 1 --branch "${REF}" "${REPO}" .

# --- Build mandatory 'nextrun' helper ---
# Fail fast if nextrun.go is missing
RUN test -f nextrun.go
RUN mkdir /build-helper
RUN mv nextrun.go /build-helper/
WORKDIR /build-helper
RUN go mod init nextrun && go get github.com/robfig/cron/v3
RUN GOOS="$TARGETOS" GOARCH="$TARGETARCH" CGO_ENABLED=0 go build -o /nextrun nextrun.go

# --- Build the main 'epgo' application ---
WORKDIR /src
RUN go mod tidy
RUN GOOS="$TARGETOS" GOARCH="$TARGETARCH" CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /epgo .

# ---------- Stage 2: Final ----------
FROM alpine:3.20
RUN apk add --no-cache tzdata su-exec dcron ca-certificates
WORKDIR /app

COPY --from=builder /epgo /usr/bin/epgo
COPY --from=builder /nextrun /usr/local/bin/nextrun
COPY --from=builder /src/sample-config.yaml /usr/local/share/sample-config.yaml

COPY entrypoint.sh /usr/local/bin/entrypoint.sh
RUN sed -i 's/\r$//' /usr/local/bin/entrypoint.sh && chmod +x /usr/local/bin/entrypoint.sh

ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
