FROM golang:1.21-alpine as builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go get .
RUN go build

FROM alpine:3.18

ARG USER=docker
ARG UID=1000
ARG GID=1000

RUN apk add --no-cache bash openrc busybox-suid

# --- Create user and group ---
RUN addgroup -g "${GID}" "${USER}" && \
    adduser -u "${UID}" -G "${USER}" -h /app -s /sbin/nologin -D "${USER}"

WORKDIR /app

COPY --from=builder --chown="${USER}:${USER}" /app/epgo /usr/local/bin/epgo
COPY --chown="${USER}:${USER}" sample-config.yaml /app/config.yaml

# --- Cron setup (Corrected) ---

# 1. Create crontab *before* switching user
#    We do this as root, so we have permissions.
RUN echo "*/5 * * * * /usr/local/bin/epgo --config /app/config.yaml >> /proc/1/fd/1 2>> /proc/1/fd/2" > /etc/crontabs/docker

# 2.  Change ownership of the crontab to the correct user.
RUN chown "${USER}:${USER}" /etc/crontabs/docker

# --- Switch to the non-root user ---
USER "${USER}"

CMD ["crond", "-f", "-l", "2"]