FROM golang:1.13 as build

ADD . /go/src/github.com/google/trillian-examples
WORKDIR /go/src/github.com/google/trillian-examples

ARG GOFLAGS=""

RUN go get -v ./gossip/minimal/gosmin

FROM gcr.io/distroless/base

COPY --from=build /go/bin/gosmin /

ENTRYPOINT ["/gosmin", "--config=/gosmin.cfg", "--metrics_endpoint=localhost:6962", "--alsologtostderr", "-v=1"]

EXPOSE 6962
