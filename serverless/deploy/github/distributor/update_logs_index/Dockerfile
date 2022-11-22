FROM golang:1.19-alpine AS build

WORKDIR /src/
COPY . ./
RUN CGO_ENABLED=0 go build -o /bin/update_logs_index .

FROM alpine

RUN apk add --no-cache bash git

COPY entrypoint.sh /entrypoint.sh
COPY --from=build /bin/update_logs_index /bin/update_logs_index

ENTRYPOINT ["/entrypoint.sh"]
