FROM golang:1.21.4-alpine3.17@sha256:f3eb584dbe0fb1b1ca9b3b4b30b89fa6f578eb8365296ee27b12be80d0de0b6c AS build

WORKDIR /src/
COPY . ./
RUN CGO_ENABLED=0 go build -o /bin/update_logs_index .

FROM alpine:3.18.4@sha256:eece025e432126ce23f223450a0326fbebde39cdf496a85d8c016293fc851978

RUN apk add --no-cache bash git

COPY entrypoint.sh /entrypoint.sh
COPY --from=build /bin/update_logs_index /bin/update_logs_index

ENTRYPOINT ["/entrypoint.sh"]
