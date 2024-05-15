FROM golang:1.22.3-bullseye@sha256:e39ea97d70f3b88bd1684c69c6937c45cf97a8a0509dd46145d7095b83c5b748 AS builder
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
FROM golang:1.22.3-bullseye@sha256:e39ea97d70f3b88bd1684c69c6937c45cf97a8a0509dd46145d7095b83c5b748

COPY --from=builder /build/ft_personality /bin/ft_personality
ENTRYPOINT ["/bin/ft_personality"]
