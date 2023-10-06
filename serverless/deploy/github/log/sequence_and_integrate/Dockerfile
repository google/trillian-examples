FROM golang:1.21.2-alpine3.17@sha256:83985c4c375761859ccd7a93fca85b18ee08ba983b3ad310407f84c087ef6ce4 AS build

WORKDIR /src/
# Note: this could be a bit surprising as folks who pinned their action to some particular version
# will end up running the latest binaries. For now this is probably "ok", but when serverless is
# moved out into its own repo (where releases can be done) we should fix this behaviour.
RUN CGO_ENABLED=0 go install github.com/google/trillian-examples/serverless/cmd/integrate@HEAD
RUN CGO_ENABLED=0 go install github.com/google/trillian-examples/serverless/cmd/sequence@HEAD

FROM alpine:3.18.4@sha256:eece025e432126ce23f223450a0326fbebde39cdf496a85d8c016293fc851978

RUN apk add --no-cache bash git

COPY entrypoint.sh /entrypoint.sh
COPY --from=build /go/bin/integrate /bin/integrate
COPY --from=build /go/bin/sequence /bin/sequence

ENTRYPOINT ["/entrypoint.sh"]
