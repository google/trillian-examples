FROM golang:1.23.2-bullseye@sha256:ebaf58d74f200c27dce0181203a1347e41b5c159c9aa0517a95bec594aabbde4 AS builder
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
FROM golang:1.23.2-bullseye@sha256:ebaf58d74f200c27dce0181203a1347e41b5c159c9aa0517a95bec594aabbde4

COPY --from=builder /build/ft_personality /bin/ft_personality
ENTRYPOINT ["/bin/ft_personality"]
