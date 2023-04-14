FROM golang:1.19-alpine AS builder
RUN apk add --no-cache gcc musl-dev

ARG GOFLAGS=""
ENV GOFLAGS=$GOFLAGS
ENV GO111MODULE=on

# Move to working directory /build
WORKDIR /build
# Copy and download dependency using go mod
COPY go.mod .
COPY go.sum .
RUN go mod download

# Copy the code into the container
COPY . .

# Build the application
RUN go build -o /build/bin/witness ./witness/golang/cmd/witness

# Build release image
FROM alpine:3.17.3

COPY --from=builder /build/bin/witness /bin/witness
ENTRYPOINT ["/bin/witness"]