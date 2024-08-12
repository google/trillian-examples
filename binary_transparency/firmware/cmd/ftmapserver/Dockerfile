FROM golang:1.22.6-bullseye@sha256:37f09f0c199a07c2e72ae2cfd758681fae0681240c71d6fad42d9d090c437c38 AS builder
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
FROM golang:1.22.6-bullseye@sha256:37f09f0c199a07c2e72ae2cfd758681fae0681240c71d6fad42d9d090c437c38

COPY --from=builder /build/ftmapserver /bin/ftmapserver
ENTRYPOINT ["/bin/ftmapserver"]
