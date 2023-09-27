FROM golang:1.21-alpine3.17@sha256:46bc9c7c1a34efd4885c06fe2cf1a92ebc0aad683731c74f8fd453da3a632483 AS build

WORKDIR /src/
COPY . ./
RUN CGO_ENABLED=0 go build -o /bin/combine_witness_signatures .

FROM alpine:3.18.3@sha256:7144f7bab3d4c2648d7e59409f15ec52a18006a128c733fcff20d3a4a54ba44a

RUN apk add --no-cache bash git

COPY entrypoint.sh /entrypoint.sh
COPY --from=build /bin/combine_witness_signatures /bin/combine_witness_signatures

ENTRYPOINT ["/entrypoint.sh"]
