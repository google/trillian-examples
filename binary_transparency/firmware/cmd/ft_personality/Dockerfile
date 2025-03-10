FROM golang:1.24.1-bullseye@sha256:aa106963247f64275bd459b6b713978f1633160da53f58115922964ab0b9eae7 AS builder
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
FROM golang:1.24.1-bullseye@sha256:aa106963247f64275bd459b6b713978f1633160da53f58115922964ab0b9eae7

COPY --from=builder /build/ft_personality /bin/ft_personality
ENTRYPOINT ["/bin/ft_personality"]
