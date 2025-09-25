# Stage 1: The "Builder"
FROM golang:1.22-alpine AS builder

RUN apk add --no-cache git
WORKDIR /src
# Clone the specific branch directly into the current directory
RUN git clone --branch docker-with-progid-fix https://github.com/nilleiz/EpGo-Docker.git .
RUN CGO_ENABLED=0 go build -o /epgo .

# --- Build the 'nextrun' utility in an isolated directory ---
RUN mkdir /build-helper
COPY nextrun.go /build-helper/
WORKDIR /build-helper
RUN go mod init nextrun && go get github.com/robfig/cron/v3
RUN CGO_ENABLED=0 go build -o /nextrun nextrun.go


# Stage 2: The "Final" Image
FROM alpine:3.20
RUN apk add --no-cache tzdata su-exec dcron
WORKDIR /app

COPY --from=builder /epgo /usr/bin/epgo
# Copy the new 'nextrun' utility from the builder stage
COPY --from=builder /nextrun /usr/local/bin/nextrun
COPY --from=builder /src/sample-config.yaml /usr/local/share/sample-config.yaml

COPY entrypoint.sh /usr/local/bin/entrypoint.sh
RUN chmod +x /usr/local/bin/entrypoint.sh

ENTRYPOINT ["entrypoint.sh"]