FROM golang:1.21.2-alpine3.17@sha256:83985c4c375761859ccd7a93fca85b18ee08ba983b3ad310407f84c087ef6ce4 AS builder

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
RUN go build ./clone/cmd/ctclone

# Build release image
FROM alpine:3.18.4@sha256:eece025e432126ce23f223450a0326fbebde39cdf496a85d8c016293fc851978

COPY --from=builder /build/ctclone /bin/ctclone
ENTRYPOINT ["/bin/ctclone"]
