FROM golang:1.21.4-alpine3.17@sha256:f3eb584dbe0fb1b1ca9b3b4b30b89fa6f578eb8365296ee27b12be80d0de0b6c AS builder

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
FROM alpine:3.18.5@sha256:34871e7290500828b39e22294660bee86d966bc0017544e848dd9a255cdf59e0

COPY --from=builder /build/ctclone /bin/ctclone
ENTRYPOINT ["/bin/ctclone"]
