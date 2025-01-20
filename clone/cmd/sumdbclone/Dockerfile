FROM golang:1.23.5-alpine3.20@sha256:def59a601e724ddac5139d447e8e9f7d0aeec25db287a9ee1615134bcda266e2 AS builder

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
RUN go build ./clone/cmd/sumdbclone

# Build release image
FROM alpine:3.21.2@sha256:56fa17d2a7e7f168a043a2712e63aed1f8543aeafdcee47c58dcffe38ed51099

COPY --from=builder /build/sumdbclone /bin/sumdbclone
ENTRYPOINT ["/bin/sumdbclone"]
