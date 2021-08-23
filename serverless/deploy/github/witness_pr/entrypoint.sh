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

    LOG_DIR="$(readlink -f -n ${INPUT_LOG_DIR})"

    # Now grab a list of all the modified/added/removed files in the PR
    FILES=$(git diff origin/master HEAD --name-only)

    local has_non_log_files=0
    local has_witnessed_checkpoint=0
    local has_log_non_witness_files=0
    declare -a witness_checkpoints

    while IFS= read -r f; do
            f=$(readlink -f -n ${f})
            if [[ ${f} = ${LOG_DIR}/checkpoint ]]; then
                echo "::error: file=${f} Modifications to checkpoint not allowed, please add files to the witnessed/ directory."
                has_log_non_witness_files=1
            elif [[ ${f} = ${LOG_DIR}/checkpoint.witnessed ]]; then
                echo "::error: file=${f} Modifications to checkpoint.witnessed not allowed, please add files to the witnessed/ directory."
                has_log_non_witness_files=1
            elif [[ ${f} = ${LOG_DIR}/witness/* ]]; then
                echo "::debug:Found entry in witness/"
                has_witnessed_checkpoint=1
                witnessed_checkpoints+=(${f})
            elif [[ ${f} = ${LOG_DIR}/* ]]; then
                echo "::debug:Added/Modified file other than checkpoint.witnessed in \`${INPUT_LOG_DIR}/\` directory"
                has_log_non_witness_files=1
            else
                echo "Found non-log file '${f}'"
                has_non_log_files=1
            fi
    done <<< ${FILES}

    if [[ ${has_witnessed_checkpoint} -eq 0 ]]; then
        echo "::debug:Nothing to do - not a witness PR"
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

    local invalid_checkpoint=0
    set +e
    for i in ${!witnessed_checkpoints[@]}; do
        /bin/validate_signatures \
            --logtostderr \
            --checkpoint="${witnessed_checkpoints[${i}]}" \
            --log_public_key="${INPUT_LOG_PUBLIC_KEY}" \
            --witness_public_key_files="${INPUT_WITNESS_KEY_FILES}" 
        if [[ $? -ne 0 ]]; then
            echo "::error: file=${witnessed_checkpoints[${i}]} Invalid witnessed checkpoint"
            invalid_checkpoint=1
        fi
    done
    set -e

    if [[ ${invalid_checkpoint} -ne 0 ]]; then
      exit 1
    fi
}

main
