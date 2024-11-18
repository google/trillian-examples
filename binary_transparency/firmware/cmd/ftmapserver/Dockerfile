FROM golang:1.23.3-bullseye@sha256:bc1b90c2a8eb0ffb62325e02a85d51031ad3afae15b3df4b6a48b7929b00badb AS builder
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
FROM golang:1.23.3-bullseye@sha256:bc1b90c2a8eb0ffb62325e02a85d51031ad3afae15b3df4b6a48b7929b00badb

COPY --from=builder /build/ftmapserver /bin/ftmapserver
ENTRYPOINT ["/bin/ftmapserver"]
