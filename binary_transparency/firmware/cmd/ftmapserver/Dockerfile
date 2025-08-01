FROM golang:1.24.5-bullseye@sha256:62ba6b19de03e891f7fa1001326bd48411f2626ff35e7ba5b9d890711ce581d9 AS builder
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
FROM golang:1.24.5-bullseye@sha256:62ba6b19de03e891f7fa1001326bd48411f2626ff35e7ba5b9d890711ce581d9

COPY --from=builder /build/ftmapserver /bin/ftmapserver
ENTRYPOINT ["/bin/ftmapserver"]
