package log

//go:generate protoc -I=. -I=$GOPATH/src -I=$GOPATH/src/github.com/googleapis/googleapis -I=$GOPATH/src/github.com/google/trillian --go_out=plugins=grpc,paths=source_relative:. log.proto
