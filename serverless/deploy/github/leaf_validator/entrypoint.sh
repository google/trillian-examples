#!/bin/bash
# This is an example leaf validator, it doesn't really do much but the idea is
# that this action would run against PRs which are effectively "queuing" leaves
# and return success/failure depending on whether the leaves present in the PR
# conform to a given set of requirements.

set -e

function main {
    if [ "${INPUT_LOG_DIR}" == "" ]; then
    echo "Missing log dir input".
    exit 1
    fi
    echo "::debug:Log directory is ${GITHUB_WORKSPACE}/${INPUT_LOG_DIR}"
    cd ${GITHUB_WORKSPACE}

    # Figure out where any leaves in the PR should be rooted under
    PENDING_DIR="$(readlink -f -n ${INPUT_LOG_DIR}/leaves/pending)"
    echo "::debug:Pending leaf directory is ${PENDING_DIR}"

    # Now grab a list of all the modified/added/removed files in the PR
    PR_NUMBER=$(echo ${GITHUB_REF} | awk 'BEGIN { FS = "/" } ; { print $3 }')
    URL="https://api.github.com/repos/${GITHUB_REPOSITORY}/pulls/${PR_NUMBER}/files"
    FILES=$(curl -s -X GET -G ${URL} | jq -r '.[] | .filename')

    # Finally, validate each of the modified/added/removed files
    while IFS= read -r f; do
        LEAF=$(readlink -f -n ${f})
        if [[ ${LEAF} = ${PENDING_DIR}/* ]]; then
            echo "::debug:Found pending leaf ${LEAF}"
            # Checks on the format/quality of the leaf could be done here, along
            # with signature verification etc.
        else
            echo "::debug:Added/Modified file outside of pending directory: ${LEAF}"
            exit 1
        fi
    done <<< ${FILES}
}

main
