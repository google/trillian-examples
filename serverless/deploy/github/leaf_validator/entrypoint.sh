#!/bin/bash
# This is an example leaf validator, it doesn't really do much but the idea is
# that this action would run against PRs which are effectively "queuing" leaves
# and return success/failure depending on whether the leaves present in the PR
# conform to a given set of requirements.
#
# We'll consider a PR good if either:
# - it only touches files outside the log directory, or
# - it only touches files in the log's leaves/pending directory.

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

    local has_non_log_files=0
    local has_log_pending_files=0
    local has_log_non_pending_files=0

    while IFS= read -r f; do
            if [[ ${LEAF} = ${PENDING_DIR}/* ]]; then
                echo "::debug:Found pending leaf ${LEAF}"
                # Checks on the format/quality of the leaf could be done here, along
                # with signature verification etc.
                has_log_pending_files=1
            elif [[ ${LEAF} = ${INPUT_LOG_DIR}/* ]]; then
                echo "::warning file=${f}::Added/Modified non-pending leaves file in \`${INPUT_LOG_DIR}/\` directory"
                has_log_non_pending_files=1
            else
                echo "Found non-log file ${f}"
                has_non_log_files=1
            fi
    done <<< ${FILES}

    if [[ (($has_log_non_pending_files)) ]]; then
        echo "::error:PR attempts to modify log structure/state"
        exit 1
    fi

    if [[ (($has_log_pending_files)) && (($has_non_log_files)) ]]; then
        echo "::error:PR mixes log additions and non-log changes, please split them up"
        exit 1
    fi
}

main
