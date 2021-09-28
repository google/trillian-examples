FROM golang AS builder

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
RUN go build -o /build/bin/feeder ./serverless/cmd/feeder/

# Build release image
FROM golang

COPY --from=builder /build/bin/feeder /bin/feeder

ENTRYPOINT ["/bin/feeder"]