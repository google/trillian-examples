#!/bin/bash
# This is an action for handling Witness submission PRs.
# 
# Witness PRs contain a modification to just one file - the log checkpoint.
#
# This action will verify that this is the case and then use the combine_signatures
# tool to validate the modification and merge the new signature(s) with the old one(s).

set -e

function main {
    cd ${GITHUB_WORKSPACE}

    LOG_DIR="$(readlink -f -n ${INPUT_PR_REPO_ROOT}/${INPUT_LOG_DIR})"
    LOG_CP="$(readlink -f -n ${INPUT_PR_REPO_ROOT}/${INPUT_LOG_DIR}/checkpoint)"
    MODIFIED_CP="$(readlink -f -n ${INPUT_PR_REPO_ROOT}/${INPUT_LOG_DIR}/checkpoint.witnessed)"

    cd ${INPUT_PR_REPO_ROOT}
    # Now grab a list of all the modified/added/removed files in the PR
    FILES=$(git diff origin/master HEAD --name-only)

    local has_non_log_files=0
    local has_witnessed_checkpoint=0
    local has_log_non_witness_files=0

    while IFS= read -r f; do
            f=$(readlink -f -n ${f})
            if [[ ${f} = ${LOG_CP} ]]; then
                echo "::error: file=${f} Modifications to checkpoint not allowed, please use checkpoint.witnessed"
                has_witnessed_checkpoint=1
            elif [[ ${f} = ${MODIFIED_CP} ]]; then
                echo "::debug:Found checkpoint.witnessed"
                has_witnessed_checkpoint=1
            elif [[ ${f} = ${LOG_DIR}/* ]]; then
                echo "::debug:Added/Modified file other than checkpoint.witnessed in \`${INPUT_LOG_DIR}/\` directory"
                has_log_non_witness_files=1
            else
                echo "Found non-log file '${f}'"
                has_non_log_files=1
            fi
    done <<< ${FILES}

    if [[ ${has_witnessed_checkpoint} -eq 0 ]]; then
        echo "::debug:Nothing to do"
        exit 0
    fi

    if [[ ${has_witnessed_checkpoint} -ne 0 && ${has_log_non_witness_files} -ne 0 ]]; then
        echo "::error:PR attempts to modify log structure/state"
        exit 1
    fi

    if [[ ${has_witnessed_checkpoint} -ne 0 && ${has_non_log_files} -ne 0 ]]; then
        echo "::error:PR mixes log witness and non-log changes, please split them up"
        exit 1
    fi

    cd ${GITHUB_WORKSPACE}

    # The checkpoint.witnessed in the PR could be either an extra signature on the master checkpoint.witnessed,
    # or it could be the first witness signature on an updated master checkpoint file.
    # The combine_signatures tool will fail if the input checkpoint bodies don't exactly match, so we can just
    # try both possibilities in order.

    set +e 

    # Attempt to merge with existing checkpoint.witnessed:
    PRISTINE_CP="$(readlink -f -n ${INPUT_PRISTINE_REPO_ROOT}/${INPUT_LOG_DIR}/checkpoint.witnessed)"
    if [ -f ${PRISTINE_CP} ]; then
        /bin/combine_signatures --log_public_key="${INPUT_LOG_PUBLIC_KEY}" --witness_public_key_files="${INPUT_PRISTINE_REPO_ROOT}/${INPUT_WITNESS_KEY_FILES}" --output=${MODIFIED_CP} ${PRISTINE_CP} ${MODIFIED_CP}
        if [ $? -eq 0 ]; then
            echo "Updated existing checkpoint.witnessed:"
            echo 
            cat ${INPUT_PR_REPO_ROOT}/${INPUT_LOG_DIR}/checkpoint.witnessed
            exit 0
        fi
    fi

    # That didn't work, so attempt to merge with the new checkpoint:
    PRISTINE_CP="$(readlink -f -n ${INPUT_PRISTINE_REPO_ROOT}/${INPUT_LOG_DIR}/checkpoint)"
    /bin/combine_signatures --log_public_key="${INPUT_LOG_PUBLIC_KEY}" --witness_public_key_files="${INPUT_PRISTINE_REPO_ROOT}/${INPUT_WITNESS_KEY_FILES}" --output=${MODIFIED_CP} ${PRISTINE_CP} ${MODIFIED_CP}
    if [ $? -eq 0 ]; then
        echo "Updated checkpoint.witnessed from checkpoint and checkpoint.witnessed:"
        echo 
        cat ${INPUT_PR_REPO_ROOT}/${INPUT_LOG_DIR}/checkpoint.witnessed
        exit 0
    fi

    echo "::error file=${MODIFIED_CP} Checkpoint is not equivalent to either log checkpoint or checkpoint.witnessed"
    exit 1

    set -e
}

main
