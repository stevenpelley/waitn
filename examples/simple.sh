#!/usr/bin/env bash

# skip if waitn is in your path
SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )
waitn_path=$(realpath "$SCRIPT_DIR/../waitn")
waitn() {
    "$waitn_path" $@
}

declare -A pids
{ sleep 1; exit 1; } &
pids[$!]=1
{ sleep 2; exit 2; } &
pids[$!]=2
{ sleep 3; exit 3; } &
pids[$!]=3

wait_for_job() {
    pid=$(waitn "${!pids[@]}")
    ret=$?
    [ $ret -eq 0 ] || { echo "bad waitn: $ret"; exit 1; }

    wait -p finished_pid $pid
    wait_ret=$?
    [ -n "$finished_pid" ] || { echo "bad wait for $pid: $finished_pid"; exit 2; }
    [ $finished_pid -eq $pid ] || { echo "pid -ne finished_pid: $pid $finished_pid"; exit 3; }

    # handle the job finishing however you like
    echo "FINISHED $val exit code $wait_ret @${SECONDS}"
    unset pids[$pid]
}

while [ ${#pids[@]} -gt 0 ]; do
    wait_for_job
done