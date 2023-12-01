FROM golang:1.21.4-alpine3.17@sha256:5ea67903a418e179d638d39a629b52169d5a2c657ea3a17950861ea481795f48 AS builder

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
FROM alpine:3.18.5@sha256:34871e7290500828b39e22294660bee86d966bc0017544e848dd9a255cdf59e0

COPY --from=builder /build/ctverify /bin/ctverify
ENTRYPOINT ["/bin/ctverify"]
