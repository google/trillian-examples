#!/bin/bash
# This is an action for combining witness cosigned checkpoints. 
# 
# Attempts to merge cosigned checkpoints from .../witness/* with either
# .../checkpoint or .../checkpoint.witnessed depending on how many
# witness signatures are available for both files.

set -e

function main {
    cd ${GITHUB_WORKSPACE}

    LOG_DIR="$(readlink -f -n ${INPUT_LOG_DIR})"
    WITNESS_DIR="${LOG_DIR}/witness"
    NUM_REQUIRED=${INPUT_NUM_WITNESS_SIGS_REQUIRED}
    CP="${LOG_DIR}/checkpoint"
    CP_WITNESSED="${LOG_DIR}/checkpoint.witnessed"

    if [[ ! -d ${WITNESS_DIR} ]]; then
    echo "::info:No witness cosigned checkpoints to combine."
        exit 0
    fi

    /bin/combine_witness_signatures \
        --logtostderr \
        --storage_dir=${LOG_DIR} \
        --log_public_key="${INPUT_LOG_PUBLIC_KEY}" \
        --witness_public_key_files="${INPUT_WITNESS_KEY_FILES}" \
        --output=${CP_WITNESSED}
}

main
