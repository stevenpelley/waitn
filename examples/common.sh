#!/usr/bin/env bash
COMMON_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )
source "$COMMON_DIR/../wait.bash"
source "$COMMON_DIR/../wait_waitn.bash"

# starts a test job and adds it to associative array "pids"
start_job() {
    local task="$1"
    local sleep_dur=$(( (($task-1)%3)+1 ))
    echo "STARTING $task, sleep $sleep_dur @${SECONDS}"
    { sleep $sleep_dur; exit $task; } &
    pids[$!]=$task
}