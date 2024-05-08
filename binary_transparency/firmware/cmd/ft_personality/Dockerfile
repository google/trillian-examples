FROM golang:1.22.3-bullseye@sha256:e72f9a1d29fbd6e1603df5a780e8f407caebef4dbb9f07536fc72f1c368298aa AS builder
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
FROM golang:1.22.3-bullseye@sha256:e72f9a1d29fbd6e1603df5a780e8f407caebef4dbb9f07536fc72f1c368298aa

COPY --from=builder /build/ft_personality /bin/ft_personality
ENTRYPOINT ["/bin/ft_personality"]
