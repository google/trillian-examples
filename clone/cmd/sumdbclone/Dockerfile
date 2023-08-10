FROM golang:1.19-alpine3.17@sha256:504f0b9b177aef633965cb55d00f37e062e9374914b4047f2e0cd260d2752791 AS builder

ARG GOFLAGS=""
ENV GOFLAGS=$GOFLAGS
ENV GO111MODULE=on

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
FROM alpine:3.18.3@sha256:7144f7bab3d4c2648d7e59409f15ec52a18006a128c733fcff20d3a4a54ba44a

COPY --from=builder /build/sumdbclone /bin/sumdbclone
ENTRYPOINT ["/bin/sumdbclone"]
