FROM golang:1.24.3-bullseye@sha256:abe2e2bb9bc0342dd1ba2f719af5c6b3859ca9ad93a7d9bcdd21310bda0327e1 AS builder
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
FROM golang:1.24.3-bullseye@sha256:abe2e2bb9bc0342dd1ba2f719af5c6b3859ca9ad93a7d9bcdd21310bda0327e1

COPY --from=builder /build/ft_personality /bin/ft_personality
ENTRYPOINT ["/bin/ft_personality"]
