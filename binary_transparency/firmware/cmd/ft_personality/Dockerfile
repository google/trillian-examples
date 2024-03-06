FROM golang:1.22.1-bullseye@sha256:9f880bd713b1e3d22d50d968e6369c6d7f750112f8e52c52b8a75756eb448d81 AS builder
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
FROM golang:1.22.1-bullseye@sha256:9f880bd713b1e3d22d50d968e6369c6d7f750112f8e52c52b8a75756eb448d81

COPY --from=builder /build/ft_personality /bin/ft_personality
ENTRYPOINT ["/bin/ft_personality"]
