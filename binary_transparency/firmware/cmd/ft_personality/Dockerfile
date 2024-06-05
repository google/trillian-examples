FROM golang:1.22.4-bullseye@sha256:0742798b94c656e75ba5409583c2485f18b52de5c9b7cf12859c945bb7414718 AS builder
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
FROM golang:1.22.4-bullseye@sha256:0742798b94c656e75ba5409583c2485f18b52de5c9b7cf12859c945bb7414718

COPY --from=builder /build/ft_personality /bin/ft_personality
ENTRYPOINT ["/bin/ft_personality"]
