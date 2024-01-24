FROM golang:1.21.6-bullseye@sha256:47fa179d4966a0950485ede2ef81567bb1cf62e1e87af07e9830e5c928d06cd0 AS builder
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
FROM golang:1.21.6-bullseye@sha256:47fa179d4966a0950485ede2ef81567bb1cf62e1e87af07e9830e5c928d06cd0

COPY --from=builder /build/ft_personality /bin/ft_personality
ENTRYPOINT ["/bin/ft_personality"]
