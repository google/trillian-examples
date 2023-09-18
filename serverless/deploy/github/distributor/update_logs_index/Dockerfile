FROM golang:1.21-alpine3.17@sha256:ecaab0e81d070800a399d2e805f0fcc22806e130166a2f880456abeb04e401b0 AS build

WORKDIR /src/
COPY . ./
RUN CGO_ENABLED=0 go build -o /bin/update_logs_index .

FROM alpine:3.18.3@sha256:7144f7bab3d4c2648d7e59409f15ec52a18006a128c733fcff20d3a4a54ba44a

RUN apk add --no-cache bash git

COPY entrypoint.sh /entrypoint.sh
COPY --from=build /bin/update_logs_index /bin/update_logs_index

ENTRYPOINT ["/entrypoint.sh"]
