#!/usr/bin/env bash
SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )

# creates a temp file, opens for writing at fd provided in name $1, reading at
# fd provided in name $2
create_temp() {
    local _waitpid_waitn_out_fd
    local _waitpid_waitn_in_fd
    tf=$(mktemp)
    exec {_waitpid_waitn_out_fd}<>$tf
    exec {_waitpid_waitn_in_fd}<>$tf
    rm $tf
    declare -n _waitpid_waitn_out_ref=$1
    _waitpid_waitn_out_ref=${_waitpid_waitn_out_fd}
    declare -n _waitpid_waitn_in_ref=$2
    _waitpid_waitn_in_ref=${_waitpid_waitn_in_fd}
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

    create_temp _waitn_out_fd _waitn_in_fd

    # run the waitn command
    wait_cmd $@ >&$_waitn_out_fd 2>&$_waitn_out_fd &
    local _waitn_waitn_pid=$!
    # wait for the waitn command
    wait $_waitn_waitn_pid
    local _waitn_wait_ret=$?

    if [ $_waitn_wait_ret -le 128 ]; then
        # waitn completed

        # get the exit code of the returned pid
        local _waitn_pid=$(wait_cmd_get_pid <&$_waitn_in_fd)

        wait $_waitn_pid
        local _waitn_ret=$?

        # assign a variable for -p if provided
        if [ -n "$_waitn_pid_var_name" ]; then
            local -g -n _waitn_pid_var_ref="$_waitn_pid_var_name"
            _waitn_pid_var_ref="$_waitn_pid"
        fi

        # wait returns the exit code of the awaited process, which is the exit
        # code of the wait builtin
        return "$_waitn_ret"
    else
        # woke up due to a signal.  We're not going to bother waiting for it.
        # BASH before 5.2 cannot use -p to distinguish between wait waking due to signal
        # and process having ended with status > 128
        kill "$_waitn_waitn_pid"

        # return the original wait_ret, which indicates which signal (128 + signal num)
        return "$_waitn_wait_ret"
    fi
}