FROM golang:1.21.5-alpine3.17@sha256:92cb87af996ec6befc85f0aec27e12ead2fab396695fa8a7abff79e021e58195 AS builder

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
RUN go build ./clone/cmd/ctclone

# Build release image
FROM alpine:3.20.1@sha256:b89d9c93e9ed3597455c90a0b88a8bbb5cb7188438f70953fede212a0c4394e0

COPY --from=builder /build/ctclone /bin/ctclone
ENTRYPOINT ["/bin/ctclone"]
