FROM golang:1.22.2-bullseye@sha256:3b55bf3986b2651a515f48ddf758b80a7d78d8be18706fd98aa74241992dac96 AS builder
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
FROM golang:1.22.2-bullseye@sha256:3b55bf3986b2651a515f48ddf758b80a7d78d8be18706fd98aa74241992dac96

COPY --from=builder /build/ft_personality /bin/ft_personality
ENTRYPOINT ["/bin/ft_personality"]
