#!/usr/bin/env bash

SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )
source $(realpath "$SCRIPT_DIR/common.sh")

# we'll sleep 2 and then kill
declare -A pids
# finishes prior to kill
{ sleep 1; exit 1; } &
pids[$!]=1
# still running when killed
{ sleep 3; exit 2; } &
pids[$!]=2

# we kill from the SIGTERM handler, so here we just skip wait waking up due to
# signal.  We're return 1 to indicate this, but we don't actually use it.
wait_for_job() {
    local finished_pid
    waitn -p finished_pid "${!pids[@]}"
    wait_ret=$?
    # this line is new relative to simple.sh
    [ -z "$finished_pid" ] && return 1
    echo "FINISHED ${pids[$finished_pid]} exit code $wait_ret @${SECONDS}"
    unset pids[$finished_pid]
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