FROM golang:1.24.1-bullseye@sha256:3c669c8fed069d80d199073b806243c4bf79ad117b797b96f18177ad9c521cff AS builder
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
FROM golang:1.24.1-bullseye@sha256:3c669c8fed069d80d199073b806243c4bf79ad117b797b96f18177ad9c521cff

COPY --from=builder /build/ft_personality /bin/ft_personality
ENTRYPOINT ["/bin/ft_personality"]
