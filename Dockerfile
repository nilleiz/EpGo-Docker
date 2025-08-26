FROM ubuntu

ARG VERSION=v2.0.1
ARG OS=epgo_linux_arm64

RUN wget https://github.com/Chuchodavids/EpGo/releases/download/${VERSION}/epgo_linux_arm64.tar.gz