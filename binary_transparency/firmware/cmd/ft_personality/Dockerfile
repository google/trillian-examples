FROM golang:1.24.4-bullseye@sha256:dfd72198d14bc22f270c9e000c304a2ffd19f5a5f693fad82643311afdc6b568 AS builder
LABEL stage=builder

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
RUN go build ./binary_transparency/firmware/cmd/ft_personality

# Build release image
FROM golang:1.24.4-bullseye@sha256:dfd72198d14bc22f270c9e000c304a2ffd19f5a5f693fad82643311afdc6b568

COPY --from=builder /build/ft_personality /bin/ft_personality
ENTRYPOINT ["/bin/ft_personality"]
