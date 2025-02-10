# This Dockerfile builds a base image for the CloudBuild integration testing.
FROM golang:1.23.6-bullseye@sha256:80eb3147ef40b58d527d9c2e634b1a79a750aee09de6f973844db38b33f0550b AS testbase

WORKDIR /testbase

ARG GOFLAGS=""
ENV GOFLAGS=$GOFLAGS

RUN echo "deb http://deb.debian.org/debian bullseye-backports main contrib non-free" >> /etc/apt/sources.list
RUN apt-get update && apt-get -y install curl docker-compose lsof netcat unzip wget xxd

RUN cd /usr/bin && curl -L -O https://github.com/stedolan/jq/releases/download/jq-1.6/jq-linux64 && mv jq-linux64 /usr/bin/jq && chmod +x /usr/bin/jq
RUN curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(go env GOPATH)/bin v1.60.1
RUN mkdir protoc && \
    (cd protoc && \
    wget "https://github.com/google/protobuf/releases/download/v3.5.1/protoc-3.5.1-linux-x86_64.zip" && \
    unzip "protoc-3.5.1-linux-x86_64.zip" \
    )

# Tamago bits
RUN apt-get -y install binutils-arm-none-eabi build-essential make u-boot-tools fuse fuse2fs
RUN curl -sfL https://github.com/usbarmory/tamago-go/releases/download/tamago-go1.23.2/tamago-go1.23.2.linux-amd64.tar.gz | tar -xzf - -C /
ENV TAMAGO=/usr/local/tamago-go/bin/go

ENV GOPATH /go
ENV PATH $GOPATH/bin:/testbase/protoc/bin:/usr/local/go/bin:/usr/local/tamago-go/bin:$PATH
