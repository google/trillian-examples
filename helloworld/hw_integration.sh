#!/bin/bash

export TREE_ID=$(go run github.com/google/trillian/cmd/createtree --admin_server=${TRILLIAN_LOG_RPC})

go test ./helloworld/. --trillian=${TRILLIAN_LOG_RPC} --tree_id=${TREE_ID} 

