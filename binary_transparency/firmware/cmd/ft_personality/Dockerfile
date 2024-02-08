FROM golang:1.22.0-bullseye@sha256:84825c0ec01d277311cfbad14321338166fc7ffa73202db3a2debfeb89e8ddd5 AS builder
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
FROM golang:1.22.0-bullseye@sha256:84825c0ec01d277311cfbad14321338166fc7ffa73202db3a2debfeb89e8ddd5

COPY --from=builder /build/ft_personality /bin/ft_personality
ENTRYPOINT ["/bin/ft_personality"]
