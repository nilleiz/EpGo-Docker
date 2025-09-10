# Use a smaller base image
FROM alpine:latest

# Install wget and other necessary tools
RUN apk add --no-cache wget tar

# Set working directory
WORKDIR /app

# Arguments for version and architecture
ARG VERSION=v3.0.1
ARG TARGETARCH=arm64

# Download and extract the release
RUN wget https://github.com/Chuchodavids/EpGo/releases/download/${VERSION}/epgo_linux_${TARGETARCH}.tar.gz && \
    tar -xvf epgo_linux_${TARGETARCH}.tar.gz && \
    rm epgo_linux_${TARGETARCH}.tar.gz

# Set the entrypoint
ENTRYPOINT ["/app/epgo"]
