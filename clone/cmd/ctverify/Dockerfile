FROM golang:1.21-alpine3.17@sha256:ecaab0e81d070800a399d2e805f0fcc22806e130166a2f880456abeb04e401b0 AS builder

ARG GOFLAGS=""
ENV GOFLAGS=$GOFLAGS

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
