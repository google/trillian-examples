FROM golang:1.21.5-alpine3.17@sha256:92cb87af996ec6befc85f0aec27e12ead2fab396695fa8a7abff79e021e58195 AS build

WORKDIR /src/
COPY . ./
RUN CGO_ENABLED=0 go build -o /bin/combine_witness_signatures .

FROM alpine:3.19.0@sha256:51b67269f354137895d43f3b3d810bfacd3945438e94dc5ac55fdac340352f48

RUN apk add --no-cache bash git

COPY entrypoint.sh /entrypoint.sh
COPY --from=build /bin/combine_witness_signatures /bin/combine_witness_signatures

ENTRYPOINT ["/entrypoint.sh"]
