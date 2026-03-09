FROM golang:1.26.0-alpine3.22@sha256:169d3991a4f795124a88b33c73549955a3d856e26e8504b5530c30bd245f9f1b AS builder

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
FROM alpine:3.23.3@sha256:25109184c71bdad752c8312a8623239686a9a2071e8825f20acb8f2198c3f659

COPY --from=builder /build/ctclone /bin/ctclone
ENTRYPOINT ["/bin/ctclone"]
