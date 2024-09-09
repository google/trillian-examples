FROM golang:1.23.1-bullseye@sha256:45b43371f21ec51276118e6806a22cbb0bca087ddd54c491fdc7149be01035d5 AS builder
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
FROM golang:1.23.1-bullseye@sha256:45b43371f21ec51276118e6806a22cbb0bca087ddd54c491fdc7149be01035d5

COPY --from=builder /build/ft_personality /bin/ft_personality
ENTRYPOINT ["/bin/ft_personality"]
