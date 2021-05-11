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
    FILES=$(git diff origin/master HEAD --name-only)

    # Finally, validate each of the modified/added/removed files
    local is_bad=0
    while IFS= read -r f; do
        LEAF=$(readlink -f -n ${f})
        if [[ ${LEAF} = ${PENDING_DIR}/* ]]; then
            echo "::debug:Found pending leaf ${LEAF}"
            # Checks on the format/quality of the leaf could be done here, along
            # with signature verification etc.
        else
            echo "::warning file=${f}::Added/Modified file outside of \`${INPUT_LOG_DIR}/leaves/pending\` directory"
            is_bad=1
        fi
    done <<< ${FILES}

    if [[ ${is_bad} -ne 0 ]]; then
        echo "::error::Found one or more invalid leaves in PR"
        exit 1
    fi
}

main
