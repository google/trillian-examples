FROM golang:1.23.3-bullseye@sha256:7ffe9d7c355928792073ba79290b68d55ee4000697e182244b892d7e292e51d6 AS builder
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
RUN go build ./binary_transparency/firmware/cmd/ft_personality

# Build release image
FROM golang:1.23.3-bullseye@sha256:7ffe9d7c355928792073ba79290b68d55ee4000697e182244b892d7e292e51d6

COPY --from=builder /build/ft_personality /bin/ft_personality
ENTRYPOINT ["/bin/ft_personality"]
