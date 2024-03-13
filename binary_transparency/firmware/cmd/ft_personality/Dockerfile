FROM golang:1.22.1-bullseye@sha256:dcff0d950cb4648fec14ee51baa76bf27db3bb1e70a49f75421a8828db7b9910 AS builder
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
FROM golang:1.22.1-bullseye@sha256:dcff0d950cb4648fec14ee51baa76bf27db3bb1e70a49f75421a8828db7b9910

COPY --from=builder /build/ft_personality /bin/ft_personality
ENTRYPOINT ["/bin/ft_personality"]
