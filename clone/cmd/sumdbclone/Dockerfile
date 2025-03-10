FROM golang:1.24.1-alpine3.20@sha256:3d9132b88a6317b846b55aa8e821821301906fe799932ecbc4f814468c6977a5 AS builder

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
RUN go build ./clone/cmd/sumdbclone

# Build release image
FROM alpine:3.21.3@sha256:a8560b36e8b8210634f77d9f7f9efd7ffa463e380b75e2e74aff4511df3ef88c

COPY --from=builder /build/sumdbclone /bin/sumdbclone
ENTRYPOINT ["/bin/sumdbclone"]
