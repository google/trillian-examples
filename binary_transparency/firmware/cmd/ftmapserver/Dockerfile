FROM golang:1.21.6-bullseye@sha256:c62751ac12cad0c514d941e36f846c1c440ca9e8ec08dd87d022fb03f0887a9b AS builder
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
FROM golang:1.21.6-bullseye@sha256:c62751ac12cad0c514d941e36f846c1c440ca9e8ec08dd87d022fb03f0887a9b

COPY --from=builder /build/ftmapserver /bin/ftmapserver
ENTRYPOINT ["/bin/ftmapserver"]
