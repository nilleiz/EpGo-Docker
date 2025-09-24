# Stage 1: The "Builder"
FROM golang:1.22-alpine AS builder

RUN apk add --no-cache git
WORKDIR /src
#ARG VERSION=v3.2.1
RUN git clone https://github.com/nilleiz/EpGo.git .
#RUN git checkout ${VERSION}
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

RUN addgroup -S app && adduser -S -G app app
RUN chown -R app:app /app

ENTRYPOINT ["entrypoint.sh"]