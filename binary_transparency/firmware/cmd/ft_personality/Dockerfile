FROM golang:1.22.5-bullseye@sha256:583d5af8289d30de50aa0dcf4985d8b8746e52622becd6e1a62cfe191d5275a5 AS builder
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
FROM golang:1.22.5-bullseye@sha256:583d5af8289d30de50aa0dcf4985d8b8746e52622becd6e1a62cfe191d5275a5

COPY --from=builder /build/ft_personality /bin/ft_personality
ENTRYPOINT ["/bin/ft_personality"]
