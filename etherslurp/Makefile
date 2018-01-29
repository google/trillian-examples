T=$(GOPATH)/src/github.com/google/trillian
E=$(GOPATH)/src/github.com/google/trillian-examples/etherslurp
G=$(GOPATH)/src/github.com/ethereum/go-ethereum/build/bin/geth

trillian::
	go get -u github.com/google/trillian
	cd $T && \
	go build ./server/trillian_log_server && \
	go build ./server/trillian_log_signer && \
	go build ./server/trillian_map_server

tlserver::
	cd $T && ./trillian_log_server --logtostderr ...

tlsigner::
	cd $T && ./trillian_log_signer --logtostderr --force_master --http_endpoint=localhost:8092 --batch_size=1000 --sequencer_guard_window=0 --sequencer_interval=200ms

tmserver::
	cd $T && ./trillian_map_server --logtostderr --rpc_endpoint=localhost:8095

createlog:: $E/logid

$E/logid:
	cd $T && go build ./cmd/createtree/ && ./createtree --admin_server=localhost:8090 > $E/logid

createmap:: $E/mapid

$E/mapid:
	cd $T && go build ./cmd/createtree/ && ./createtree --admin_server=localhost:8095 --tree_type=MAP --hash_strategy=TEST_MAP_HASHER > $E/mapid

$G:
	go get -u github.com/ethereum/go-ethereum
	cd ${GOPATH}/src/github.com/ethereum/go-ethereum && make geth

geth:: $G

$E/rinkeby.json:
	wget https://www.rinkeby.io/rinkeby.json

initgeth:: $G $E/rinkeby.json
	$G --datadir=$HOME/rinkeby init $E/rinkeby.json

rungeth::
	$G --networkid=4 --datadir=$HOME/rinkeby --cache=1024 --syncmode=full --verbosity 3 --ethstats='yournode:Respect my authoritah!@stats.rinkeby.io' --bootnodes=enode://a24ac7c5484ef4ed0c5eb2d36620ba4e4aa13b8c84684e1b4aab0cebea2ae45cb4d375b77eab56516d34bfbd3c1a833fc51296ff084b770b94fb9028c4d25ccf@52.169.42.101:30303 --rpc console

follower::
	go run ./cmd/follower/main.go --geth=http://127.0.0.1:8545 --trillian_log=localhost:8090 --log_id `cat logid` --logtostderr

mapper::
	go run ./cmd/mapper/main.go --logtostderr --trillian_log=localhost:8090 --log_id `cat logid` --map_id `cat mapid`

ui::
	go run ./cmd/ui/main.go --logtostderr --trillian_map=localhost:8095 --map_id=`cat mapid`
