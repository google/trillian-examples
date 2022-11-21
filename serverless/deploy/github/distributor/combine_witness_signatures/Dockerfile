FROM golang:1.19-alpine AS build

WORKDIR /src/
COPY . ./
RUN CGO_ENABLED=0 go build -o /bin/combine_witness_signatures .

FROM alpine

RUN apk add --no-cache bash git

COPY entrypoint.sh /entrypoint.sh
COPY --from=build /bin/combine_witness_signatures /bin/combine_witness_signatures

ENTRYPOINT ["/entrypoint.sh"]
