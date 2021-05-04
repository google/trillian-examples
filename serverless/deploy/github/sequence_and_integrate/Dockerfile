FROM golang:1.16-alpine AS build

WORKDIR /src/
COPY . ./
RUN CGO_ENABLED=0 go build -o /bin/integrate github.com/google/trillian-examples/serverless/cmd/integrate
RUN CGO_ENABLED=0 go build -o /bin/sequence github.com/google/trillian-examples/serverless/cmd/sequence

FROM alpine

RUN apk add --no-cache bash git

COPY entrypoint.sh /entrypoint.sh
COPY --from=build /bin/integrate /bin/integrate
COPY --from=build /bin/sequence /bin/sequence

ENTRYPOINT ["/entrypoint.sh"]
