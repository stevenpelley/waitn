#!/usr/bin/env bash

# skip if waitn is in your path
SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )
waitn_path=$(realpath "$SCRIPT_DIR/../waitn")
waitn() {
    "$waitn_path" $@
}

declare -A pids
limit=3
remaining_tasks=($(seq 0 9))

# modifies remaining_tasks and pids
# returns 0 if a new task was started and 1 otherwise.
# this does not test the concurrency limit
start_task_if_remaining() {
    [ "${#remaining_tasks[@]}" -eq 0 ] && return 1
    task=${remaining_tasks[0]}
    remaining_tasks=("${remaining_tasks[@]:1}")
    # give some different sleep durations
    sleep_dur=$(( ($task%3)+1 ))
    echo "STARTING $task, sleep $sleep_dur @${SECONDS}"
    { sleep $sleep_dur; exit $task; } &
    pids[$!]=$task
    return 0
}

# identical to simple.sh
wait_for_job() {
    pid=$(waitn "${!pids[@]}")
    ret=$?
    [ $ret -eq 0 ] || { echo "bad waitn: $ret"; exit 1; }

    wait -p finished_pid $pid
    wait_ret=$?
    [ -n "$finished_pid" ] || { echo "bad wait for $pid: $finished_pid"; exit 2; }
    [ $finished_pid -eq $pid ] || { echo "pid -ne finished_pid: $pid $finished_pid"; exit 3; }

    # handle the job finishing however you like
    val="${pids[$pid]}"
    echo "FINISHED $val exit code $wait_ret @${SECONDS}"
    unset pids[$pid]
}

while [ ${#remaining_tasks[@]} -gt 0 ] || [ ${#pids[@]} -gt 0 ]; do
    # start processes until we get up to the limit
    while [ ${#pids[@]} -lt $limit ] && start_task_if_remaining ; do : ; done
    wait_for_job
done