FROM golang:1.21.6-bullseye@sha256:adf7ccb07fe8ccadf7bb0317f02d2c3a4916f824a23f6975fd36c4bd7feece3f AS builder
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
FROM golang:1.21.6-bullseye@sha256:adf7ccb07fe8ccadf7bb0317f02d2c3a4916f824a23f6975fd36c4bd7feece3f

COPY --from=builder /build/ftmapserver /bin/ftmapserver
ENTRYPOINT ["/bin/ftmapserver"]
