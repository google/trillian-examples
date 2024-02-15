FROM golang:1.22.0-bullseye@sha256:0c099cf97c6162ceab8c9494c4ce8c2a19a73b852f334ec1876a42d05eb5f90b AS builder
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
FROM golang:1.22.0-bullseye@sha256:0c099cf97c6162ceab8c9494c4ce8c2a19a73b852f334ec1876a42d05eb5f90b

COPY --from=builder /build/ft_personality /bin/ft_personality
ENTRYPOINT ["/bin/ft_personality"]
