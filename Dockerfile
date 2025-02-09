FROM golang:1.20-alpine as builder

# Create a working directory inside the builder stage *before* copying
WORKDIR /app

# Copy go.mod and go.sum FIRST, and download dependencies.  This is key for caching.
COPY go.mod go.sum ./
RUN go mod download  # Download dependencies *before* copying the rest of the code

# Now copy the source code
COPY *.go ./

# No need for go mod init here, it should be done before you build the Dockerfile

RUN go build

# --- Runtime Stage ---
FROM alpine:3.18

# Use ARG for build-time variables, then ENV for runtime
ARG USER=docker
ARG UID=1000
ARG GID=1000

# Combine addgroup and adduser for efficiency. Use standard user/group creation best practices.
RUN addgroup -g "${GID}" "${USER}" && \
    adduser -u "${UID}" -G "${USER}" -h /app -s /sbin/nologin -D "${USER}"

# /app is already owned by root, so no need to create and chown initially
WORKDIR /app

# Copy the binary and config.  Crucially, use the correct paths.
COPY --from=builder --chown="${USER}:${USER}" /app/guide2go /usr/local/bin/guide2go
COPY --chown="${USER}:${USER}" sample-config.yaml /app/config.yaml

USER "${USER}"

# Correct CMD with explicit path and config file
CMD ["/usr/local/bin/guide2go", "--config", "/app/config.yaml"]