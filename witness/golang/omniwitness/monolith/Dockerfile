FROM golang:buster AS builder

ARG GOFLAGS=""
ENV GOFLAGS=$GOFLAGS
ENV GO111MODULE=on

# Move to working directory /build
WORKDIR /build

# Copy and download dependency using go mod
COPY go.mod .
COPY go.sum .
RUN go mod download

# Copy the code into the container
COPY . .

# Build the application
RUN go build -o omniwitness ./witness/golang/omniwitness/monolith

# Build release image
FROM golang:buster

COPY --from=builder /build/omniwitness /bin/omniwitness
ENTRYPOINT ["/bin/omniwitness"]
