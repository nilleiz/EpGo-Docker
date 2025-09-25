# Stage 1: The "Builder"
FROM golang:1.22-alpine AS builder

RUN apk add --no-cache git
WORKDIR /src
# Clone the specific branch directly into the current directory
RUN git clone --branch add-back-aspect https://github.com/nilleiz/EpGo-Docker.git .

# --- Isolate and Build the 'nextrun' utility FIRST ---
RUN mkdir /build-helper
# Move the helper tool's source code out of the main project directory
RUN mv nextrun.go /build-helper/
WORKDIR /build-helper
RUN go mod init nextrun && go get github.com/robfig/cron/v3
RUN CGO_ENABLED=0 go build -o /nextrun nextrun.go

# --- Build the main 'epgo' application ---
WORKDIR /src
# Download dependencies for the main application
RUN go mod tidy
# Build the main application now that the helper source is gone
RUN CGO_ENABLED=0 go build -o /epgo .


# Stage 2: The "Final" Image
FROM alpine:3.20
RUN apk add --no-cache tzdata su-exec dcron
WORKDIR /app

COPY --from=builder /epgo /usr/bin/epgo
COPY --from=builder /nextrun /usr/local/bin/nextrun
COPY --from=builder /src/sample-config.yaml /usr/local/share/sample-config.yaml

COPY entrypoint.sh /usr/local/bin/entrypoint.sh
RUN chmod +x /usr/local/bin/entrypoint.sh

# User creation is now handled by the entrypoint script at runtime
ENTRYPOINT ["entrypoint.sh"]