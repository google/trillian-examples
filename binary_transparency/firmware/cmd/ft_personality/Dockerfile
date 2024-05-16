FROM golang:1.22.3-bullseye@sha256:78b171fe51f25b8c3197710f281dacae94759a254e9a486576005f9dadba9e7d AS builder
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
FROM golang:1.22.3-bullseye@sha256:78b171fe51f25b8c3197710f281dacae94759a254e9a486576005f9dadba9e7d

COPY --from=builder /build/ft_personality /bin/ft_personality
ENTRYPOINT ["/bin/ft_personality"]
