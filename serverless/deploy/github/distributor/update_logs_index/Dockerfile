FROM golang:1.21-alpine3.17@sha256:6467daf26aec6b82c750be4ca9b6aaf3a6a07ed22e1d4ff72dbdda9a93bb39bf AS build

WORKDIR /src/
COPY . ./
RUN CGO_ENABLED=0 go build -o /bin/update_logs_index .

FROM alpine:3.17.3@sha256:124c7d2707904eea7431fffe91522a01e5a861a624ee31d03372cc1d138a3126

RUN apk add --no-cache bash git

COPY entrypoint.sh /entrypoint.sh
COPY --from=build /bin/update_logs_index /bin/update_logs_index

ENTRYPOINT ["/entrypoint.sh"]
