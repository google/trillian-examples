#!/bin/bash

set -e

function main {
    if [ "${SERVERLESS_LOG_PRIVATE_KEY}" == "" ]; then
        echo "Missing log private key."
        exit 1
    fi
    if [ "${INPUT_LOG_DIR}" == "" ]; then
        echo "Missing log dir input."
        exit 1
    fi
    echo "::debug:Log directory is ${GITHUB_WORKSPACE}/${INPUT_LOG_DIR}"

    cd ${GITHUB_WORKSPACE}

    if [ ! -f "${INPUT_LOG_DIR}/checkpoint" ]; then
        echo "::debug:No checkpoint file - initialising log"
        if [ -z "${INPUT_ECOSYSTEM}" ]; then 
            /bin/integrate --storage_dir="${INPUT_LOG_DIR}" --initialise --logtostderr
        else
            /bin/integrate --storage_dir="${INPUT_LOG_DIR}" --ecosystem="${INPUT_ECOSYSTEM}" --initialise --logtostderr
        fi

        exit
    fi

    PENDING="${INPUT_LOG_DIR}/leaves/pending"
    if [ ! -d "${PENDING}" ]; then
        echo "::debug:Nothing to do :("
        exit
    fi

    echo "::debug:Sequencing..."
    /bin/sequence --storage_dir="${INPUT_LOG_DIR}" --logtostderr --entries "${PENDING}/*"
    rm ${PENDING}/*

    echo "::debug:Integrating..."
    if [ -z "${INPUT_ECOSYSTEM}" ]; then 
        /bin/integrate --storage_dir="${INPUT_LOG_DIR}" --logtostderr
    else
        /bin/integrate --storage_dir="${INPUT_LOG_DIR}" --ecosystem="${INPUT_ECOSYSTEM}" --logtostderr
    fi
}

main
