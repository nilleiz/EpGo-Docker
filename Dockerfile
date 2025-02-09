FROM golang:1.21-alpine as builder

# Create a working directory inside the builder stage *before* copying
WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download  # Download dependencies *before* copying the rest of the code

COPY . . 

RUN go get .
RUN go build

FROM alpine:3.18

ARG USER=docker
ARG UID=1000
ARG GID=1000

RUN addgroup -g "${GID}" "${USER}" && \
    adduser -u "${UID}" -G "${USER}" -h /app -s /sbin/nologin -D "${USER}"

WORKDIR /app

COPY --from=builder --chown="${USER}:${USER}" /app/guide2go /usr/local/bin/guide2go
COPY --chown="${USER}:${USER}" sample-config.yaml /app/config.yaml

USER "${USER}"

CMD ["/usr/local/bin/guide2go", "--config", "/app/config.yaml"]