#!/bin/bash
# This is an action for combining witness cosigned checkpoints. 
 
set -e

function main {
    cd ${GITHUB_WORKSPACE}

    /bin/combine_witness_signatures \
        --logtostderr \
        --distributor_dir=${INPUT_DISTRIBUTOR_DIR} \
        --config=${INPUT_CONFIG} \
        --dry_run=${INPUT_DRY_RUN}
}

main
