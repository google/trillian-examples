# This Dockerfile builds a base image for the CloudBuild integration testing.
FROM ubuntu as testbase

WORKDIR /testbase

ARG GOFLAGS=""
ENV GOFLAGS=$GOFLAGS
ENV GO111MODULE=on

RUN apt-get update && apt-get -y install curl docker-compose lsof unzip wget xxd

RUN curl -sfL https://golang.org/dl/go1.15.6.linux-amd64.tar.gz | tar -xzf - -C /usr/local

RUN cd /usr/bin && curl -L -O https://github.com/stedolan/jq/releases/download/jq-1.6/jq-linux64 && mv jq-linux64 /usr/bin/jq && chmod +x /usr/bin/jq
RUN curl -sfL https://install.goreleaser.com/github.com/golangci/golangci-lint.sh | sh -s -- -b $(go env GOPATH)/bin v1.18.0
RUN mkdir protoc && \
    (cd protoc && \
    wget "https://github.com/google/protobuf/releases/download/v3.5.1/protoc-3.5.1-linux-x86_64.zip" && \
    unzip "protoc-3.5.1-linux-x86_64.zip" \
    )

# Tamago bits
RUN apt-get -y install binutils-arm-none-eabi build-essential fuse fuse2fs make u-boot-tools
RUN curl -sfL https://github.com/f-secure-foundry/tamago-go/releases/download/tamago-go1.15.6/tamago-go1.15.6.linux-amd64.tar.gz | tar -xzf - -C /
ENV TAMAGO=/usr/local/tamago-go/bin/go

ENV GOPATH /go
ENV PATH $GOPATH/bin:/testbase/protoc/bin:/usr/local/tamago-go/bin:/usr/local/go/bin:$PATH
