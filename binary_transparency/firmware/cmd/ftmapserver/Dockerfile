FROM golang:1.24.0-bullseye@sha256:f44db136b8d4822a059d9b6256be67bd233a19d39a88c46b1023efe3d848931b AS builder
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
RUN go build ./binary_transparency/firmware/cmd/ftmapserver

# Build release image
FROM golang:1.24.0-bullseye@sha256:f44db136b8d4822a059d9b6256be67bd233a19d39a88c46b1023efe3d848931b

COPY --from=builder /build/ftmapserver /bin/ftmapserver
ENTRYPOINT ["/bin/ftmapserver"]
