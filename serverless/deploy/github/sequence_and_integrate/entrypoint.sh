#!/bin/bash

set -e

function main {
    if [ "${INPUT_LOG_DIR}" == "" ]; then
    echo "Missing log dir input".
    exit 1
    fi
    echo "::debug:Log directory is ${GITHUB_WORKSPACE}/${INPUT_LOG_DIR}"

    cd ${GITHUB_WORKSPACE}

    PENDING="${INPUT_LOG_DIR}/leaves/pending"
    if [ ! -d "${PENDING}" ]; then
        echo "::debug:Nothing to do :("
        exit
    fi

    if [ ! -f "${INPUT_LOG_DIR}/checkpoint" ]; then
        echo "::debug:No checkpoint file - initialising log"
        /bin/integrate --storage_dir="${INPUT_LOG_DIR}" --initialise --logtostderr
    fi

    echo "::debug:Sequencing..."
    /bin/sequence --storage_dir="${INPUT_LOG_DIR}" --logtostderr --entries "${PENDING}/*"
    rm ${PENDING}/*

    echo "::debug:Integrating..."
    /bin/integrate --storage_dir="${INPUT_LOG_DIR}" ${SEQUENCE_FLAGS} --logtostderr
}

main
