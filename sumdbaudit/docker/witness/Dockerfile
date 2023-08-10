FROM golang:1.20.3-buster@sha256:73c225bc5e2353f20dbe0466819b70a51a114a93bfe4af035a3bb9e1ecdd4107 AS builder

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
RUN go build -o sumdbwitness ./sumdbaudit/cli/witness

# Build release image
FROM golang:1.20.3-buster@sha256:73c225bc5e2353f20dbe0466819b70a51a114a93bfe4af035a3bb9e1ecdd4107

COPY --from=builder /build/sumdbwitness /bin/witness
ENTRYPOINT ["/bin/witness"]
