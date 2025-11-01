# This Dockerfile builds a base image for the CloudBuild integration testing.
FROM golang:1.25.3-bookworm@sha256:ee420c17fa013f71eca6b35c3547b854c838d4f26056a34eb6171bba5bf8ece4 AS testbase

WORKDIR /testbase

ARG GOFLAGS=""
ENV GOFLAGS=$GOFLAGS

RUN echo "deb http://deb.debian.org/debian bookworm-backports main contrib non-free" >> /etc/apt/sources.list
RUN apt-get update && apt-get -y install curl docker-compose lsof netcat-traditional unzip wget xxd

RUN cd /usr/bin && curl -L -O https://github.com/stedolan/jq/releases/download/jq-1.6/jq-linux64 && mv jq-linux64 /usr/bin/jq && chmod +x /usr/bin/jq
RUN mkdir protoc && \
    (cd protoc && \
    wget "https://github.com/google/protobuf/releases/download/v3.5.1/protoc-3.5.1-linux-x86_64.zip" && \
    unzip "protoc-3.5.1-linux-x86_64.zip" \
    )

# Tamago bits
RUN apt-get -y install binutils-arm-none-eabi build-essential make u-boot-tools fuse fuse2fs
RUN curl -sfL https://github.com/usbarmory/tamago-go/releases/download/tamago-go1.25.1/tamago-go1.25.1.linux-amd64.tar.gz | tar -xzf - -C /
ENV TAMAGO=/usr/local/tamago-go/bin/go

ENV GOPATH /go
ENV PATH $GOPATH/bin:/testbase/protoc/bin:/usr/local/go/bin:/usr/local/tamago-go/bin:$PATH
