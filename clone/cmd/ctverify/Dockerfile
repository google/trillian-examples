FROM golang:1.24.0-alpine3.20@sha256:79f7ffeff943577c82791c9e125ab806f133ae3d8af8ad8916704922ddcc9dd8 AS builder

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
FROM alpine:3.21.3@sha256:a8560b36e8b8210634f77d9f7f9efd7ffa463e380b75e2e74aff4511df3ef88c

COPY --from=builder /build/ctverify /bin/ctverify
ENTRYPOINT ["/bin/ctverify"]
