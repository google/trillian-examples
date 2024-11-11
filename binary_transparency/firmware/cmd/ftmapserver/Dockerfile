FROM golang:1.23.3-bullseye@sha256:1002103155eb5802f6876dceb8cb8bf965cc6d3deeb34f11e4774926aca6a1fd AS builder
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
FROM golang:1.23.3-bullseye@sha256:1002103155eb5802f6876dceb8cb8bf965cc6d3deeb34f11e4774926aca6a1fd

COPY --from=builder /build/ftmapserver /bin/ftmapserver
ENTRYPOINT ["/bin/ftmapserver"]
