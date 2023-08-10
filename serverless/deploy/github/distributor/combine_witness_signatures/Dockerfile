FROM golang:1.19-alpine3.17@sha256:504f0b9b177aef633965cb55d00f37e062e9374914b4047f2e0cd260d2752791 AS build

WORKDIR /src/
COPY . ./
RUN CGO_ENABLED=0 go build -o /bin/combine_witness_signatures .

FROM alpine:3.18.3@sha256:7144f7bab3d4c2648d7e59409f15ec52a18006a128c733fcff20d3a4a54ba44a

RUN apk add --no-cache bash git

COPY entrypoint.sh /entrypoint.sh
COPY --from=build /bin/combine_witness_signatures /bin/combine_witness_signatures

ENTRYPOINT ["/entrypoint.sh"]
