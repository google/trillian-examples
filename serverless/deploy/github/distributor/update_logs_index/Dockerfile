FROM golang:1.21.5-alpine3.17@sha256:92cb87af996ec6befc85f0aec27e12ead2fab396695fa8a7abff79e021e58195 AS build

WORKDIR /src/
COPY . ./
RUN CGO_ENABLED=0 go build -o /bin/update_logs_index .

FROM alpine:3.18.5@sha256:34871e7290500828b39e22294660bee86d966bc0017544e848dd9a255cdf59e0

RUN apk add --no-cache bash git

COPY entrypoint.sh /entrypoint.sh
COPY --from=build /bin/update_logs_index /bin/update_logs_index

ENTRYPOINT ["/entrypoint.sh"]
