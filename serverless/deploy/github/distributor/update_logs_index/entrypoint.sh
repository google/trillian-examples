#!/bin/bash
# This is an action for generating a markdown index file for the logs/ directory.
 
set -e

function main {
    cd ${GITHUB_WORKSPACE}

    /bin/update_logs_index \
        --logtostderr \
        --distributor_dir=${INPUT_DISTRIBUTOR_DIR} \
        --config=${INPUT_CONFIG} \
	--output=${INPUT_OUTPUT}
}

main
