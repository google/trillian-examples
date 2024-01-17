FROM golang:1.21.6-bullseye@sha256:8d22d45396a5a19a3edeefd48ec23eb1d6b1bcf2205fe84caab41c2321f4374e AS builder
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
FROM golang:1.21.6-bullseye@sha256:8d22d45396a5a19a3edeefd48ec23eb1d6b1bcf2205fe84caab41c2321f4374e

COPY --from=builder /build/ft_personality /bin/ft_personality
ENTRYPOINT ["/bin/ft_personality"]
