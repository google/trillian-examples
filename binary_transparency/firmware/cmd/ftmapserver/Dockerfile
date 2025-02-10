FROM golang:1.23.6-bullseye@sha256:80eb3147ef40b58d527d9c2e634b1a79a750aee09de6f973844db38b33f0550b AS builder
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
FROM golang:1.23.6-bullseye@sha256:80eb3147ef40b58d527d9c2e634b1a79a750aee09de6f973844db38b33f0550b

COPY --from=builder /build/ftmapserver /bin/ftmapserver
ENTRYPOINT ["/bin/ftmapserver"]
