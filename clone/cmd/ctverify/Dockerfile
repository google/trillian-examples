FROM golang:1.21-alpine3.17@sha256:6467daf26aec6b82c750be4ca9b6aaf3a6a07ed22e1d4ff72dbdda9a93bb39bf AS builder

ARG GOFLAGS=""
ENV GOFLAGS=$GOFLAGS
ENV GO111MODULE=on

# Move to working directory /build
WORKDIR /build

# Copy and download dependency using go mod
COPY go.mod .
COPY go.sum .
RUN go mod download

# Copy the code into the container
COPY . .

# Build the application
RUN go build ./clone/cmd/ctverify

# Build release image
FROM alpine:3.18.3@sha256:7144f7bab3d4c2648d7e59409f15ec52a18006a128c733fcff20d3a4a54ba44a

COPY --from=builder /build/ctverify /bin/ctverify
ENTRYPOINT ["/bin/ctverify"]
