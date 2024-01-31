#!/usr/bin/env bash
#
# intended to be sourced by examples

# you can skip if waitn is in PATH
SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )
_waitn_path=$(realpath "$SCRIPT_DIR/../waitn")

# set up a temp file for waitn output, as well as an EXIT trap to rm it
# make sure to save and run any existing EXIT trap
_cur="$(trap -p EXIT | sed -nr "s/trap -- '(.*)' EXIT\$/\1/p")"
on_exit() {
    [ -n "$_waitn_temp_file" ] && rm -f "$_waitn_temp_file"
    [ -n "$_cur" ] && $_cur
}
trap on_exit EXIT
_waitn_temp_file=$(mktemp)

# set _waitn_path or if waitn is in PATH just call it
waitn_cmd() {
    "$_waitn_path" $@
}

# intended to match bash's "wait -n -p VARNAME pids..."
# do not pass "-n" flag
#
# note that we must take care with namerefs and locals.  If the "pid_var_name"
# passed to us aliases with any local var here then we end up setting the local
# instead of a global.
#
# all local variables will be prefixed with _waitn_.  The caller must not pass
# such a variable with -p and doing so will result in an error.
#
# rename this if the waitn command is in PATH
waitn() {
    # parse out any -p option
    local _waitn_pid_var_name
    if [ "$1" = "-p" ]; then
        _waitn_pid_var_name="$2"
        if [[ $myvar =~ ^_waitn_ ]]; then
            echo "-p varname begins with _waitn_.  Such names are restricted to prevent nameref collisions.  Use a different name"
            exit 1
        fi
        unset -n _waitn_pid_var_name
        shift 2
    fi

    # run the waitn command
    waitn_cmd $@ > $_waitn_temp_file &
    local _waitn_waitn_pid=$!
    local _waitn_finished_pid
    # wait for the waitn command
    wait -p _waitn_finished_pid $_waitn_waitn_pid
    local _waitn_wait_ret=$?

    if [ -n "$_waitn_finished_pid" ]; then
        # waitn completed

        # get the exit code of the returned pid
        local _waitn_pid=$(cat "$_waitn_temp_file")
        local _waitn_double_check_pid
        wait -p _waitn_double_check_pid $_waitn_pid
        local _waitn_ret=$?
        # assert that we found the same pid
        if [ -z "$_waitn_double_check_pid" ] || [ "$_waitn_double_check_pid" != "$_waitn_pid" ]; then
            echo "waiting to get exit code failed. pid: $_waitn_pid. found: $_waitn_double_check_pid"
            exit 1;
        fi

        # assign a variable for -p if provided
        if [ -n $_waitn_pid_var_name ]; then
            local -g -n _waitn_pid_var_ref="$_waitn_pid_var_name"
            _waitn_pid_var_ref="$_waitn_pid"
        fi

        # wait returns the exit code of the awaited process, which is the exit
        # code of the wait builtin
        return "$_waitn_ret"
    else
        # woke up due to a signal
        kill "$_waitn_waitn_pid"
        # wait until waitn definitely returns.  I want to reuse the temp file
        # without having to recreate it.  We can't risk the old waitn being
        # around and overwriting the file.
        while true; do
            wait -p _waitn_finished_pid $_waitn_waitn_pid
            [ -n "$_waitn_finished_pid" ] && break
        done

        # return the original wait_ret, which indicates which signal (128 + signal num)
        return "$_waitn_wait_ret"
    fi
}

# starts a test job and adds it to associative array "pids"
start_job() {
    local task="$1"
    local sleep_dur=$(( (($task-1)%3)+1 ))
    echo "STARTING $task, sleep $sleep_dur @${SECONDS}"
    { sleep $sleep_dur; exit $task; } &
    pids[$!]=$task
}