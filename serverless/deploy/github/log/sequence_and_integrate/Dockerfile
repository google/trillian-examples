FROM golang:1.19-alpine3.17@sha256:504f0b9b177aef633965cb55d00f37e062e9374914b4047f2e0cd260d2752791 AS build

WORKDIR /src/
# Note: this could be a bit surprising as folks who pinned their action to some particular version
# will end up running the latest binaries. For now this is probably "ok", but when serverless is
# moved out into its own repo (where releases can be done) we should fix this behaviour.
RUN CGO_ENABLED=0 go install github.com/google/trillian-examples/serverless/cmd/integrate@HEAD
RUN CGO_ENABLED=0 go install github.com/google/trillian-examples/serverless/cmd/sequence@HEAD

FROM alpine:3.18.3@sha256:7144f7bab3d4c2648d7e59409f15ec52a18006a128c733fcff20d3a4a54ba44a

RUN apk add --no-cache bash git

COPY entrypoint.sh /entrypoint.sh
COPY --from=build /go/bin/integrate /bin/integrate
COPY --from=build /go/bin/sequence /bin/sequence

ENTRYPOINT ["/entrypoint.sh"]
