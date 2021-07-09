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

    MODIFIED_CP="$(readlink -f -n ${INPUT_PR_REPO_ROOT}/${INPUT_LOG_DIR}/checkpoint)"
    PRISTINE_CP="$(readlink -f -n ${INPUT_PRISTINE_REPO_ROOT}/${INPUT_LOG_DIR}/checkpoint)"

    cd ${INPUT_PR_REPO_ROOT}
    # Now grab a list of all the modified/added/removed files in the PR
    FILES=$(git diff origin/master HEAD --name-only)

    local has_non_log_files=0
    local has_checkpoint=0
    local has_log_non_checkpoint_files=0

    while IFS= read -r f; do
            FILE=$(readlink -f -n ${f})
            if [[ ${FILE} = ${MODIFIED_CP} ]]; then
                echo "::debug:Found checkpoint ${FILE}"
                has_checkpoint=1
            elif [[ ${FILE} = ${INPUT_LOG_DIR}/* ]]; then
                echo "::warning file=${f}::Added/Modified non-checkpoint file in \`${INPUT_LOG_DIR}/\` directory"
                has_log_non_checkpoint_files=1
            else
                echo "Found non-log file ${f}"
                has_non_log_files=1
            fi
    done <<< ${FILES}

    if [[ ${has_log_non_checkpoint_files} -ne 0 ]]; then
        echo "::error:PR attempts to modify log structure/state"
        exit 1
    fi

    if [[ ${has_checkpoint} -ne 0 && ${has_non_log_files} -ne 0 ]]; then
        echo "::error:PR mixes log checkpoint and non-log changes, please split them up"
        exit 1
    fi

    cd ${GITHUB_WORKSPACE}

    echo "-- master Checkpoint: ----------"
    cat ${INPUT_PRISTINE_REPO_ROOT}/${INPUT_LOG_DIR}/checkpoint
    echo "---Witnessed Checkpoint: -------"
    cat ${INPUT_PR_REPO_ROOT}/${INPUT_LOG_DIR}/checkpoint
    echo "--------------------------------"

    /bin/combine_signatures --logtostderr --log_public_key="${INPUT_LOG_PUBLIC_KEY}" --witness_public_key_files="${INPUT_PRISTINE_REPO_ROOT}/${INPUT_WITNESS_KEY_FILES}" --output=${MODIFIED_CP} ${PRISTINE_CP} ${MODIFIED_CP}

    echo "---Merged Checkpoint: -------"
    cat ${INPUT_PR_REPO_ROOT}/${INPUT_LOG_DIR}/checkpoint
}

main
