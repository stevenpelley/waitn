#!/usr/bin/env bash

SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )
source "$SCRIPT_DIR/common.sh"

declare -A pids
start_job 1

waitn -p finished_pid "${!pids[@]}"
echo "finished pid: $finished_pid exit: $? @${SECONDS}"