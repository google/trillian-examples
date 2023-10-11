FROM golang:1.21.3-alpine3.17@sha256:a5588b9dd0b185020a9acf8471c536304d78af57f3a5c1b474bdb0bc90734a4a AS build

WORKDIR /src/
COPY . ./
RUN CGO_ENABLED=0 go build -o /bin/combine_witness_signatures .

FROM alpine:3.18.4@sha256:eece025e432126ce23f223450a0326fbebde39cdf496a85d8c016293fc851978

RUN apk add --no-cache bash git

COPY entrypoint.sh /entrypoint.sh
COPY --from=build /bin/combine_witness_signatures /bin/combine_witness_signatures

ENTRYPOINT ["/entrypoint.sh"]
