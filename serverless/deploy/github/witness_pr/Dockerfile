FROM golang:1.16-alpine AS build

WORKDIR /src/
COPY . ./
RUN CGO_ENABLED=0 go build -o /bin/combine_signatures github.com/google/trillian-examples/serverless/cmd/combine_signatures

FROM alpine

RUN apk add --no-cache bash git rsync

COPY entrypoint.sh /entrypoint.sh
COPY --from=build /bin/combine_signatures /bin/combine_signatures

ENTRYPOINT ["/entrypoint.sh"]