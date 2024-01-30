#!/usr/bin/env bash

# skip if waitn is in your path
SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )
waitn_path=$(realpath "$SCRIPT_DIR/../waitn")
waitn() {
    "$waitn_path" $@
}

# we'll sleep 2 and then kill
declare -A pids
# finishes prior to kill
{ sleep 1; exit 1; } &
pids[$!]=1
# still running when killed
{ sleep 3; exit 2; } &
pids[$!]=2

# assume temp file name is written to $waitn_file
on_exit() {
    [ -n "$waitn_file" ] && rm -f "$waitn_file"
}
trap on_exit EXIT
# waitn writes its result to this temp file
waitn_file=$(mktemp)

wait_for_job() {
    # we must run waitn in the bg and await it in order to allow the signal
    # handler to run
    waitn "${!pids[@]}" > $waitn_file &
    waitn_pid=$!
    while : ; do
        unset finished_waitn_pid
        wait -p finished_waitn_pid $waitn_pid
        ret=$?
        [ -n "$finished_waitn_pid" ] && break
        # otherwise we woke and the trap handler ran
    done
    [ $ret -eq 0 ] || { echo "bad waitn: $ret"; exit 1; }
    pid=$(cat "$waitn_file")

    # the rest is the same as simple.sh
    wait -p finished_pid $pid
    wait_ret=$?
    [ -n "$finished_pid" ] || { echo "bad wait for $pid: $finished_pid"; exit 2; }
    [ $finished_pid -eq $pid ] || { echo "pid -ne finished_pid: $pid $finished_pid"; exit 3; }

    # handle the job finishing however you like
    echo "FINISHED $val exit code $wait_ret @${SECONDS}"
    unset pids[$pid]
}

handled_term=false
term_handler() {
    handled_term=true
    echo "killing jobs from handler @${SECONDS}"
    kill -TERM "${!pids[@]}"
}
trap term_handler TERM

sleep 2 && echo "killing bash! @${SECONDS}" && kill -TERM $$ &

while [ ${#pids[@]} -gt 0 ]; do
    wait_for_job
done

if $handled_term; then
    # reset
    trap - TERM
    # term self after having forwarded and joining all children
    kill -TERM $$
fi

# bash should print "Terminated" and end.
# you can also query $? for 143 (128 + 15 where SIGTERM is 15)