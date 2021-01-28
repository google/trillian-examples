FROM golang:1.15 AS builder

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
RUN go build ./sumdbaudit/cli/mirror

# Build release image
FROM golang:1.15-buster

COPY --from=builder /build/mirror /bin/mirror
ENTRYPOINT ["/bin/mirror"]
