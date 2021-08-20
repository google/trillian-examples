FROM golang:1.16-alpine AS build

WORKDIR /src/
COPY . ./
RUN CGO_ENABLED=0 go build -o /bin/validate_signatures .

FROM alpine

RUN apk add --no-cache bash git

COPY entrypoint.sh /entrypoint.sh
COPY --from=build /bin/validate_signatures /bin/validate_signatures

ENTRYPOINT ["/entrypoint.sh"]