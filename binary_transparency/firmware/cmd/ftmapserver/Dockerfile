FROM golang:1.23.0-bullseye@sha256:ecef8303ced05b7cd1addf3c8ea98974f9231d4c5a0c230d23b37bb623714a23 AS builder
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
FROM golang:1.23.0-bullseye@sha256:ecef8303ced05b7cd1addf3c8ea98974f9231d4c5a0c230d23b37bb623714a23

COPY --from=builder /build/ftmapserver /bin/ftmapserver
ENTRYPOINT ["/bin/ftmapserver"]
