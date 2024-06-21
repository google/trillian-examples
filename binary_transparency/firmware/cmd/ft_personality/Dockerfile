FROM golang:1.22.4-bullseye@sha256:067c5c7fe6d79f900c5ebe8351166356d6e3bbfcc6f807030e89b9a929252273 AS builder
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
FROM golang:1.22.4-bullseye@sha256:067c5c7fe6d79f900c5ebe8351166356d6e3bbfcc6f807030e89b9a929252273

COPY --from=builder /build/ft_personality /bin/ft_personality
ENTRYPOINT ["/bin/ft_personality"]
