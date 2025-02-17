FROM golang:1.24.0-bullseye@sha256:cf29cafe674ad5e637311148fb7933f67c4e8cafa79ce066aad0e7aa708fc287 AS builder
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
FROM golang:1.24.0-bullseye@sha256:cf29cafe674ad5e637311148fb7933f67c4e8cafa79ce066aad0e7aa708fc287

COPY --from=builder /build/ftmapserver /bin/ftmapserver
ENTRYPOINT ["/bin/ftmapserver"]
