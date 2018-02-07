T=$(GOPATH)/src/github.com/google/trillian
R=$(GOPATH)/src/github.com/google/trillian-examples/registers

trillian::
	go get -u github.com/google/trillian
	cd $T && \
	go build ./server/trillian_log_server && \
	go build ./server/trillian_log_signer && \
	go build ./server/trillian_map_server

tlserver::
	cd $T && ./trillian_log_server --logtostderr ...

createlog:: $R/logid

$R/logid:
	cd $T && go build ./cmd/createtree/ && ./createtree --admin_server=localhost:8090 > $R/logid

deletelog::
	cd $T && go run cmd/deletetree/main.go --admin_server=localhost:8090 --log_id=`cat $R/logid`
	rm $R/logid
