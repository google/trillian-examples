FROM golang:1.16-alpine AS build

WORKDIR /src/
COPY . ./
RUN CGO_ENABLED=0 go build -o /bin/verify_signatures .

FROM alpine

RUN apk add --no-cache bash git

COPY entrypoint.sh /entrypoint.sh
COPY --from=build /bin/verify_signatures /bin/verify_signatures

ENTRYPOINT ["/entrypoint.sh"]