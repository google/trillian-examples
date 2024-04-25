FROM golang:1.22.2-bullseye@sha256:72885e2245d6bcc63af0538043c63454878a22733587af87a4cfb12268d03baf AS builder
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
RUN go build ./binary_transparency/firmware/cmd/ftmapserver

# Build release image
FROM golang:1.22.2-bullseye@sha256:72885e2245d6bcc63af0538043c63454878a22733587af87a4cfb12268d03baf

COPY --from=builder /build/ftmapserver /bin/ftmapserver
ENTRYPOINT ["/bin/ftmapserver"]
