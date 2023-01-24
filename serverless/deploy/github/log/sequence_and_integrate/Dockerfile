FROM golang:1.19-alpine AS build

WORKDIR /src/
RUN CGO_ENABLED=0 go install github.com/google/trillian-examples/serverless/cmd/integrate@HEAD
RUN CGO_ENABLED=0 go install github.com/google/trillian-examples/serverless/cmd/sequence@HEAD

FROM alpine

RUN apk add --no-cache bash git

COPY entrypoint.sh /entrypoint.sh
COPY --from=build /go/bin/integrate /bin/integrate
COPY --from=build /go/bin/sequence /bin/sequence

ENTRYPOINT ["/entrypoint.sh"]
