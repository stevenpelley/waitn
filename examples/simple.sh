#!/usr/bin/env bash

# skip if waitn is in your path
SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )
waitn_path=$(realpath "$SCRIPT_DIR/../waitn")
waitn() {
    $waitn_path $@
}

# call #! immediately afterwards to get the job pid
create_job() {
    sleep_dur=$1
    exit_code=$2
    { sleep $sleep_dur; exit $exit_code; } &
}

declare -A pids
{ sleep 1; exit 1; } &
pids[$!]=1
{ sleep 2; exit 2; } &
pids[$!]=2
{ sleep 3; exit 3; } &
pids[$!]=3

while [ ${#pids[@]} -gt 0 ]; do
    pid=$(waitn "${!pids[@]}")
    ret=$?
    [ $ret -eq 0 ] || { echo "bad waitn: $ret"; exit 1; }

    wait -p finished_pid $pid
    wait_ret=$?
    [ -n "$finished_pid" ] || { echo "bad wait for $pid: $finished_pid"; exit 2; }
    [ $finished_pid -eq $pid ] || { echo "pid -ne finished_pid: $pid $finished_pid"; exit 3; }

    unset pids[$pid]
    # handle the job finishing however you like
    echo "pid $pid val ${pids[$pid]} exit code $wait_ret @${SECONDS}"
done