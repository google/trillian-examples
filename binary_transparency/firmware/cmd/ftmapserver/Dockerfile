FROM golang:1.22.5-bullseye@sha256:11ff2c5de8044d8d749cc684f6f3cdfbc9b2755960a62d4948b3483dc090e7aa AS builder
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
FROM golang:1.22.5-bullseye@sha256:11ff2c5de8044d8d749cc684f6f3cdfbc9b2755960a62d4948b3483dc090e7aa

COPY --from=builder /build/ftmapserver /bin/ftmapserver
ENTRYPOINT ["/bin/ftmapserver"]
