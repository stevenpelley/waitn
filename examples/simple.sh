#!/usr/bin/env bash

SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )
source $(realpath "$SCRIPT_DIR/common.sh")

declare -A pids
# start jobs, populate $pids
for task in {1..3}; do
    start_job $task
done

wait_for_job() {
    local finished_pid
    waitn -p finished_pid "${!pids[@]}"
    wait_ret=$?
    echo "FINISHED ${pids[$finished_pid]} exit code $wait_ret @${SECONDS}"
    unset pids[$finished_pid]
}

while [ ${#pids[@]} -gt 0 ]; do
    wait_for_job
done