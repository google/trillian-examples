FROM golang:1.23.4-bullseye@sha256:0c1625adb190cfc734a71d8e1350bc73ce29a0a8081849411a155fe5c9aaf302 AS builder
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
FROM golang:1.23.4-bullseye@sha256:0c1625adb190cfc734a71d8e1350bc73ce29a0a8081849411a155fe5c9aaf302

COPY --from=builder /build/ftmapserver /bin/ftmapserver
ENTRYPOINT ["/bin/ftmapserver"]
