# This Dockerfile builds a base image for the CloudBuild integration testing.
# It just extends a golang/linux base image with a few extra tools which the
# tests need.
FROM golang:1.15 as testbase

WORKDIR /testbase

ARG GOFLAGS=""
ENV GOFLAGS=$GOFLAGS
ENV GO111MODULE=on

RUN apt-get update && apt-get -y install curl unzip wget default-mysql-client docker-compose lsof xxd
RUN cd /usr/bin && curl -L -O https://github.com/stedolan/jq/releases/download/jq-1.6/jq-linux64 && mv jq-linux64 /usr/bin/jq && chmod +x /usr/bin/jq
RUN curl -sfL https://install.goreleaser.com/github.com/golangci/golangci-lint.sh | sh -s -- -b $(go env GOPATH)/bin v1.18.0
RUN mkdir protoc && \
    (cd protoc && \
    wget "https://github.com/google/protobuf/releases/download/v3.5.1/protoc-3.5.1-linux-x86_64.zip" && \
    unzip "protoc-3.5.1-linux-x86_64.zip" \
    )
RUN pwd

ENV GOPATH /go
ENV PATH $GOPATH/bin:/testbase/protoc/bin:$PATH
